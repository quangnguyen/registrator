package consul

import (
	"fmt"
	"github.com/hashicorp/go-cleanhttp"
	log "log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	consul "github.com/hashicorp/consul/api"
	"github.com/quangnguyen/registrator/bridge"
)

const DefaultInterval = "10s"

func init() {
	f := new(Factory)
	bridge.Register(f, "consul")
	bridge.Register(f, "consul-tls")
	bridge.Register(f, "consul-unix")
}

func (r *Consul) interpolateService(script string, service *bridge.Service) string {
	withIp := strings.Replace(script, "$SERVICE_IP", service.IP, -1)
	withPort := strings.Replace(withIp, "$SERVICE_PORT", strconv.Itoa(service.Port), -1)
	return withPort
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	config := consul.DefaultConfig()
	aclToken := os.Getenv("CONSUL_ACL_TOKEN")
	if aclToken != "" {
		config.Token = aclToken
	}

	if uri.Scheme == "consul-unix" {
		config.Address = strings.TrimPrefix(uri.String(), "consul-")
	} else if uri.Scheme == "consul-tls" {
		tlsConfigDesc := &consul.TLSConfig{
			Address:            uri.Host,
			CAFile:             os.Getenv("CONSUL_CACERT"),
			CertFile:           os.Getenv("CONSUL_CLIENT_CERT"),
			KeyFile:            os.Getenv("CONSUL_CLIENT_KEY"),
			InsecureSkipVerify: false,
		}
		tlsConfig, err := consul.SetupTLSConfig(tlsConfigDesc)
		if err != nil {
			log.Error("Cannot set up Consul TLSConfig", "error", err)
		}
		config.Scheme = "https"
		transport := cleanhttp.DefaultPooledTransport()
		transport.TLSClientConfig = tlsConfig
		config.Transport = transport
		config.Address = uri.Host
	} else if uri.Host != "" {
		config.Address = uri.Host
	}
	client, err := consul.NewClient(config)
	if err != nil {
		log.Error("consul: ", "scheme", uri.Scheme, "error", err)
	}
	return &Consul{client: client}
}

type Consul struct {
	client *consul.Client
}

// Ping will try to connect to consul by attempting to retrieve the current leader.
func (r *Consul) Ping() error {
	status := r.client.Status()
	leader, err := status.Leader()
	if err != nil {
		return err
	}
	log.Info("consul: current leader", "leader", leader)

	return nil
}

func (r *Consul) Register(service *bridge.Service) error {
	registration := consul.AgentServiceRegistration{
		ID:      service.ID,
		Name:    service.Name,
		Port:    service.Port,
		Tags:    service.Tags,
		Address: service.IP,
		Check:   r.buildCheck(service),
		Meta:    service.Attrs,
	}

	opts := consul.ServiceRegisterOpts{
		ReplaceExistingChecks: true,
	}

	return r.client.Agent().ServiceRegisterOpts(&registration, opts)
}

func (r *Consul) buildCheck(service *bridge.Service) *consul.AgentServiceCheck {
	check := new(consul.AgentServiceCheck)
	if status := service.Attrs["check_initial_status"]; status != "" {
		check.Status = status
	}
	if path := service.Attrs["check_http"]; path != "" {
		check.HTTP = fmt.Sprintf("http://%s:%d%s", service.IP, service.Port, path)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
		if method := service.Attrs["check_http_method"]; method != "" {
			check.Method = method
		}
	} else if path := service.Attrs["check_https"]; path != "" {
		check.HTTP = fmt.Sprintf("https://%s:%d%s", service.IP, service.Port, path)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
		if method := service.Attrs["check_https_method"]; method != "" {
			check.Method = method
		}
	} else if cmd := service.Attrs["check_cmd"]; cmd != "" {
		check.Args = []string{"check-cmd", service.Origin.ContainerID[:12], service.Origin.ExposedPort, cmd}
	} else if script := service.Attrs["check_script"]; script != "" {
		check.Args = []string{r.interpolateService(script, service)}
	} else if ttl := service.Attrs["check_ttl"]; ttl != "" {
		check.TTL = ttl
	} else if tcp := service.Attrs["check_tcp"]; tcp != "" {
		check.TCP = fmt.Sprintf("%s:%d", service.IP, service.Port)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
	} else if grpc := service.Attrs["check_grpc"]; grpc != "" {
		check.GRPC = fmt.Sprintf("%s:%d", service.IP, service.Port)
		if timeout := service.Attrs["check_timeout"]; timeout != "" {
			check.Timeout = timeout
		}
		if useTLS := service.Attrs["check_grpc_use_tls"]; useTLS != "" {
			check.GRPCUseTLS = true
			if tlsSkipVerify := service.Attrs["check_tls_skip_verify"]; tlsSkipVerify != "" {
				check.TLSSkipVerify = true
			}
		}
	} else {
		return nil
	}
	if len(check.Args) != 0 || check.HTTP != "" || check.TCP != "" || check.GRPC != "" {
		if interval := service.Attrs["check_interval"]; interval != "" {
			check.Interval = interval
		} else {
			check.Interval = DefaultInterval
		}
	}
	if deregisterAfter := service.Attrs["check_deregister_after"]; deregisterAfter != "" {
		check.DeregisterCriticalServiceAfter = deregisterAfter
	}
	return check
}

func (r *Consul) Deregister(service *bridge.Service) error {
	return r.client.Agent().ServiceDeregister(service.ID)
}

func (r *Consul) Refresh(_ *bridge.Service) error {
	return nil
}

func (r *Consul) Services() ([]*bridge.Service, error) {
	services, err := r.client.Agent().Services()
	if err != nil {
		return []*bridge.Service{}, err
	}
	out := make([]*bridge.Service, len(services))
	i := 0
	for _, v := range services {
		s := &bridge.Service{
			ID:   v.ID,
			Name: v.Service,
			Port: v.Port,
			Tags: v.Tags,
			IP:   v.Address,
		}
		out[i] = s
		i++
	}
	return out, nil
}
