package consul

import (
	log "log/slog"
	"net"
	"net/url"
	"strconv"
	"strings"

	consul "github.com/hashicorp/consul/api"
	"github.com/quangnguyen/registrator/bridge"
)

func init() {
	f := new(Factory)
	bridge.Register(f, "consulkv")
	bridge.Register(f, "consulkv-unix")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	config := consul.DefaultConfig()
	path := uri.Path
	if uri.Scheme == "consulkv-unix" {
		spl := strings.SplitN(uri.Path, ":", 2)
		config.Address, path = "unix://"+spl[0], spl[1]
	} else if uri.Host != "" {
		config.Address = uri.Host
	}
	client, err := consul.NewClient(config)
	if err != nil {
		log.Error("consulkv: ", "error", err)
	}
	return &ConsulKV{client: client, path: path}
}

type ConsulKV struct {
	client *consul.Client
	path   string
}

// Ping will try to connect to consul by attempting to retrieve the current leader.
func (r *ConsulKV) Ping() error {
	status := r.client.Status()
	leader, err := status.Leader()
	if err != nil {
		return err
	}
	log.Info("consulkv: current leader ", "leader", leader)

	return nil
}

func (r *ConsulKV) Register(service *bridge.Service) error {
	log.Info("Register")
	path := r.path[1:] + "/" + service.Name + "/" + service.ID
	port := strconv.Itoa(service.Port)
	addr := net.JoinHostPort(service.IP, port)
	log.Info("path", "path", path)
	_, err := r.client.KV().Put(&consul.KVPair{Key: path, Value: []byte(addr)}, nil)
	if err != nil {
		log.Error("consulkv: failed to register service", "error", err)
	}
	return err
}

func (r *ConsulKV) Deregister(service *bridge.Service) error {
	path := r.path[1:] + "/" + service.Name + "/" + service.ID
	_, err := r.client.KV().Delete(path, nil)
	if err != nil {
		log.Error("consulkv: failed to deregister service", "error", err)
	}
	return err
}

func (r *ConsulKV) Refresh(_ *bridge.Service) error {
	return nil
}

func (r *ConsulKV) Services() ([]*bridge.Service, error) {
	return []*bridge.Service{}, nil
}
