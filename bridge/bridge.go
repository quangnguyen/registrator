package bridge

import (
	"context"
	"errors"
	log "log/slog"
	"net"
	"net/url"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
)

var serviceIDPattern = regexp.MustCompile(`^(.+?):([a-zA-Z0-9][a-zA-Z0-9_.-]+):[0-9]+(?::udp)?$`)

type Bridge struct {
	sync.Mutex
	registry       RegistryAdapter
	docker         *client.Client
	services       map[string][]*Service
	deadContainers map[string]*DeadContainer
	config         Config
}

func New(docker *client.Client, adapterUri string, config Config) (*Bridge, error) {
	uri, err := url.Parse(adapterUri)
	if err != nil {
		return nil, errors.New("bad adapter uri: " + adapterUri)
	}
	factory, found := AdapterFactories.Lookup(uri.Scheme)
	if !found {
		return nil, errors.New("unrecognized adapter: " + adapterUri)
	}

	log.Info("Using adapter", "scheme", uri.Scheme, "uri", uri)
	return &Bridge{
		docker:         docker,
		config:         config,
		registry:       factory.New(uri),
		services:       make(map[string][]*Service),
		deadContainers: make(map[string]*DeadContainer),
	}, nil
}

func (b *Bridge) Ping() error {
	return b.registry.Ping()
}

func (b *Bridge) Add(containerId string) {
	b.Lock()
	defer b.Unlock()
	b.add(containerId, false)
}

func (b *Bridge) Remove(containerId string) {
	b.remove(containerId, true)
}

func (b *Bridge) RemoveOnExit(containerId string) {
	b.remove(containerId, b.shouldRemove(containerId))
}

func (b *Bridge) Refresh() {
	b.Lock()
	defer b.Unlock()

	for containerId, deadContainer := range b.deadContainers {
		deadContainer.TTL -= b.config.RefreshInterval
		if deadContainer.TTL <= 0 {
			delete(b.deadContainers, containerId)
		}
	}

	for containerId, services := range b.services {
		for _, service := range services {
			err := b.registry.Refresh(service)
			if err != nil {
				log.Error("refresh failed", "serviceID", service.ID, "error", err)
				continue
			}
			log.Info("refreshed service", "containerID", containerId[:12], "serviceID", service.ID)
		}
	}
}

func (b *Bridge) Sync(quiet bool) {
	b.Lock()
	defer b.Unlock()

	ctx := context.Background()
	containers, err := b.docker.ContainerList(ctx, container.ListOptions{})
	if err != nil && quiet {
		log.Error("error listing containers, skipping sync")
		return
	} else if err != nil {
		log.Error("fatal error", "error", err)
		return
	}

	log.Info("Syncing services", "containerCount", len(containers))

	for _, listing := range containers {
		services := b.services[listing.ID]
		if services == nil {
			b.add(listing.ID, quiet)
		} else {
			for _, service := range services {
				err := b.registry.Register(service)
				if err != nil {
					log.Error("sync register failed", "service", service, "error", err)
				}
			}
		}
	}

	if b.config.Cleanup {
		log.Info("Listing non-exited containers")
		filterArgs := filters.NewArgs()
		filterArgs.Add("status", "created")
		filterArgs.Add("status", "restarting")
		filterArgs.Add("status", "running")
		filterArgs.Add("status", "paused")
		nonExitedContainers, err := b.docker.ContainerList(ctx, container.ListOptions{Filters: filterArgs})
		if err != nil {
			log.Error("error listing nonExitedContainers, skipping sync", "error", err)
			return
		}
		for listingId := range b.services {
			found := false
			for _, container := range nonExitedContainers {
				if listingId == container.ID {
					found = true
					break
				}
			}
			if !found {
				log.Info("stale: Removing service because it does not exist", "serviceID", listingId)
				go b.RemoveOnExit(listingId)
			}
		}

		log.Info("Cleaning up dangling services")
		extServices, err := b.registry.Services()
		if err != nil {
			log.Error("cleanup failed", "error", err)
			return
		}

	Outer:
		for _, extService := range extServices {
			matches := serviceIDPattern.FindStringSubmatch(extService.ID)
			if len(matches) != 3 {
				continue
			}
			serviceHostname := matches[1]
			if serviceHostname != Hostname {
				continue
			}
			serviceContainerName := matches[2]
			for _, listing := range b.services {
				for _, service := range listing {
					if service.Name == extService.Name && serviceContainerName == service.Origin.container.Name[1:] {
						continue Outer
					}
				}
			}
			log.Info("dangling service", "serviceID", extService.ID)
			err := b.registry.Deregister(extService)
			if err != nil {
				log.Error("deregister failed", "serviceID", extService.ID, "error", err)
				continue
			}
			log.Info("service removed", "serviceID", extService.ID)
		}
	}
}

func (b *Bridge) add(containerId string, quiet bool) {
	if d := b.deadContainers[containerId]; d != nil {
		b.services[containerId] = d.Services
		delete(b.deadContainers, containerId)
	}

	if b.services[containerId] != nil {
		log.Info("container already exists, ignoring", "containerID", containerId[:12])
		return
	}

	ctx := context.Background()
	container, err := b.docker.ContainerInspect(ctx, containerId)
	if err != nil {
		log.Error("unable to inspect container", "containerID", containerId[:12], "error", err)
		return
	}

	ports := make(map[string]ServicePort)

	for port := range container.Config.ExposedPorts {
		published := []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: port.Port()}}
		ports[string(port)] = servicePort(container, port, published)
	}

	for port, published := range container.NetworkSettings.Ports {
		ports[string(port)] = servicePort(container, port, published)
	}

	if len(ports) == 0 && !quiet {
		log.Info("ignored: no published ports", "containerID", container.ID[:12])
		return
	}

	servicePorts := make(map[string]ServicePort)
	for key, port := range ports {
		if b.config.Internal != true && port.HostPort == "" {
			if !quiet {
				log.Info("ignored: port not published on host", "containerID", container.ID[:12], "port", port.ExposedPort)
			}
			continue
		}
		servicePorts[key] = port
	}

	isGroup := len(servicePorts) > 1
	for _, port := range servicePorts {
		service := b.newService(port, isGroup)
		if service == nil {
			if !quiet {
				log.Info("ignored: service on port", "containerID", container.ID[:12], "port", port.ExposedPort)
			}
			continue
		}
		err := b.registry.Register(service)
		if err != nil {
			log.Error("register failed", "service", service, "error", err)
			continue
		}
		b.services[container.ID] = append(b.services[container.ID], service)
		log.Info("added service", "containerID", container.ID[:12], "serviceID", service.ID)
	}
}

func (b *Bridge) newService(port ServicePort, isGroup bool) *Service {
	container := port.container
	defaultName := strings.Split(path.Base(container.Config.Image), ":")[0]

	hostname := Hostname
	if hostname == "" {
		hostname = port.HostIP
	}
	if port.HostIP == "0.0.0.0" {
		ip, err := net.ResolveIPAddr("ip", hostname)
		if err == nil {
			port.HostIP = ip.String()
		}
	}

	if b.config.HostIp != "" {
		port.HostIP = b.config.HostIp
	}

	metadata, metadataFromPort := serviceMetaData(container.Config, port.ExposedPort)

	ignore := mapDefault(metadata, "ignore", "")
	if ignore != "" {
		return nil
	}

	serviceName := mapDefault(metadata, "name", "")
	if serviceName == "" {
		if b.config.Explicit {
			return nil
		}
		serviceName = defaultName
	}

	service := new(Service)
	service.Origin = port
	service.ID = hostname + ":" + container.Name[1:] + ":" + port.ExposedPort
	service.Name = serviceName
	if isGroup && !metadataFromPort["name"] {
		service.Name += "-" + port.ExposedPort
	}
	var p int

	if b.config.Internal == true {
		service.IP = port.ExposedIP
		p, _ = strconv.Atoi(port.ExposedPort)
	} else {
		service.IP = port.HostIP
		p, _ = strconv.Atoi(port.HostPort)
	}
	service.Port = p

	if b.config.UseIpFromLabel != "" {
		containerIp := container.Config.Labels[b.config.UseIpFromLabel]
		if containerIp != "" {
			slashIndex := strings.LastIndex(containerIp, "/")
			if slashIndex > -1 {
				service.IP = containerIp[:slashIndex]
			} else {
				service.IP = containerIp
			}
			log.Info("using container IP from label", "ip", service.IP, "label", b.config.UseIpFromLabel)
		} else {
			log.Info("label not found in container configuration", "label", b.config.UseIpFromLabel)
		}
	}

	networkMode := container.HostConfig.NetworkMode
	if !networkMode.IsNone() {
		if networkMode.IsContainer() {
			networkContainerId := strings.Split(string(networkMode), ":")[1]
			log.Info("detected container NetworkMode, linked to", "networkContainerID", networkContainerId[:12])
			ctx := context.Background()
			networkContainer, err := b.docker.ContainerInspect(ctx, networkContainerId)
			if err != nil {
				log.Error("unable to inspect network container", "networkContainerID", networkContainerId[:12], "error", err)
			} else {
				service.IP = networkContainer.NetworkSettings.IPAddress
				log.Info("using network container IP", "ip", service.IP)
			}
		}
	}

	if port.ExposedPortProtocol == "udp" {
		service.Tags = combineTags(mapDefault(metadata, "tags", ""), b.config.ForceTags, "udp")
		service.ID = service.ID + ":udp"
	} else {
		service.Tags = combineTags(mapDefault(metadata, "tags", ""), b.config.ForceTags)
	}

	id := mapDefault(metadata, "id", "")
	if id != "" {
		service.ID = id
	}

	delete(metadata, "id")
	delete(metadata, "tags")
	delete(metadata, "name")
	service.Attrs = metadata
	service.TTL = b.config.RefreshTtl

	return service
}

func (b *Bridge) remove(containerId string, deregister bool) {
	b.Lock()
	defer b.Unlock()

	if deregister {
		deregisterAll := func(services []*Service) {
			for _, service := range services {
				err := b.registry.Deregister(service)
				if err != nil {
					log.Error("deregister failed", "serviceID", service.ID, "error", err)
					continue
				}
				log.Info("removed service", "containerID", containerId[:12], "serviceID", service.ID)
			}
		}
		deregisterAll(b.services[containerId])
		if d := b.deadContainers[containerId]; d != nil {
			deregisterAll(d.Services)
			delete(b.deadContainers, containerId)
		}
	} else if b.config.RefreshTtl != 0 && b.services[containerId] != nil {
		b.deadContainers[containerId] = &DeadContainer{b.config.RefreshTtl, b.services[containerId]}
	}
	delete(b.services, containerId)
}

var dockerSignaledBit = 128

func (b *Bridge) shouldRemove(containerId string) bool {
	if b.config.DeregisterCheck == "always" {
		return true
	}

	ctx := context.Background()
	inspectResult, err := b.docker.ContainerInspect(ctx, containerId)

	switch {
	case err != nil:
		log.Error("error fetching status for inspectResult on \"die\" event", "containerID", containerId[:12], "error", err)
		return false
	case inspectResult.State.Running:
		log.Info("not removing inspectResult, still running", "containerID", containerId[:12])
		return false
	case inspectResult.State.ExitCode == 0:
		return true
	case inspectResult.State.ExitCode&dockerSignaledBit == dockerSignaledBit:
		return true
	}
	return false
}

var Hostname string

func init() {
	Hostname, _ = os.Hostname()
}
