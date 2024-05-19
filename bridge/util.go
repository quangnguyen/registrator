package bridge

import (
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"strconv"
	"strings"

	"github.com/cenkalti/backoff"
)

func retry(fn func() error) error {
	return backoff.Retry(fn, backoff.NewExponentialBackOff())
}

func mapDefault(m map[string]string, key, default_ string) string {
	v, ok := m[key]
	if !ok || v == "" {
		return default_
	}
	return v
}

// Golang regexp module does not support /(?!\\),/ syntax for spliting by not escaped comma
// Then this function is reproducing it
func recParseEscapedComma(str string) []string {
	if len(str) == 0 {
		return []string{}
	} else if str[0] == ',' {
		return recParseEscapedComma(str[1:])
	}

	offset := 0
	for len(str[offset:]) > 0 {
		index := strings.Index(str[offset:], ",")

		if index == -1 {
			break
		} else if str[offset+index-1:offset+index] != "\\" {
			return append(recParseEscapedComma(str[offset+index+1:]), str[:offset+index])
		}

		str = str[:offset+index-1] + str[offset+index:]
		offset += index
	}

	return []string{str}
}

func combineTags(tagParts ...string) []string {
	tags := make([]string, 0)
	for _, element := range tagParts {
		tags = append(tags, recParseEscapedComma(element)...)
	}
	return tags
}

func serviceMetaData(config *container.Config, port string) (map[string]string, map[string]bool) {
	meta := config.Env
	for k, v := range config.Labels {
		meta = append(meta, k+"="+v)
	}
	metadata := make(map[string]string)
	metadataFromPort := make(map[string]bool)
	for _, kv := range meta {
		kvp := strings.SplitN(kv, "=", 2)
		if strings.HasPrefix(kvp[0], "SERVICE_") && len(kvp) > 1 {
			key := strings.ToLower(strings.TrimPrefix(kvp[0], "SERVICE_"))
			if metadataFromPort[key] {
				continue
			}
			portkey := strings.SplitN(key, "_", 2)
			_, err := strconv.Atoi(portkey[0])
			if err == nil && len(portkey) > 1 {
				if portkey[0] != port {
					continue
				}
				metadata[portkey[1]] = kvp[1]
				metadataFromPort[portkey[1]] = true
			} else {
				metadata[key] = kvp[1]
			}
		}
	}
	return metadata, metadataFromPort
}

func servicePort(containerJSON types.ContainerJSON, port nat.Port, portBindings []nat.PortBinding) ServicePort {
	var hostPort, hostIP, exposedPort, exposedPortProtocol, exposedIP string
	if len(portBindings) > 0 {
		hostPort = portBindings[0].HostPort
		hostIP = portBindings[0].HostIP
	}
	if hostIP == "" {
		hostIP = "0.0.0.0"
	}

	//for overlay networks
	//detect if container use overlay network, then set HostIP into NetworkSettings.Network[string].IPAddress
	//better to use registrator with -internal flag
	nm := containerJSON.HostConfig.NetworkMode
	if !nm.IsBridge() && !nm.IsDefault() && !nm.IsHost() {
		hostIP = containerJSON.NetworkSettings.Networks[nm.NetworkName()].IPAddress
	}

	portProtocol := strings.Split(string(port), "/")
	exposedPort = portProtocol[0]
	if len(portProtocol) == 2 {
		exposedPortProtocol = portProtocol[1]
	} else {
		exposedPortProtocol = "tcp" // default
	}

	// Nir: support docker NetworkSettings
	exposedIP = containerJSON.NetworkSettings.IPAddress
	if exposedIP == "" {
		for _, network := range containerJSON.NetworkSettings.Networks {
			exposedIP = network.IPAddress
		}
	}

	return ServicePort{
		HostPort:            hostPort,
		HostIP:              hostIP,
		ExposedPort:         exposedPort,
		ExposedIP:           exposedIP,
		ExposedPortProtocol: exposedPortProtocol,
		ContainerID:         containerJSON.ID,
		ContainerHostname:   containerJSON.Config.Hostname,
		container:           &containerJSON,
	}
}
