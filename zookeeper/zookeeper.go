package zookeeper

import (
	"encoding/json"
	log "log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/quangnguyen/registrator/bridge"
	"github.com/samuel/go-zookeeper/zk"
)

func init() {
	bridge.Register(new(Factory), "zookeeper")
}

type Factory struct{}

func (f *Factory) New(uri *url.URL) bridge.RegistryAdapter {
	c, _, err := zk.Connect([]string{uri.Host}, time.Second*10)
	if err != nil {
		panic(err)
	}
	exists, _, err := c.Exists(uri.Path)
	if err != nil {
		log.Error("zookeeper: error checking if base path exists", "error", err)
	}
	if !exists {
		c.Create(uri.Path, []byte{}, 0, zk.WorldACL(zk.PermAll))
	}
	return &Zookeeper{client: c, path: uri.Path}
}

type Zookeeper struct {
	client *zk.Conn
	path   string
}

type ZnodeBody struct {
	Name        string
	IP          string
	PublicPort  int
	PrivatePort int
	ContainerID string
	Tags        []string
	Attrs       map[string]string
}

func (r *Zookeeper) Register(service *bridge.Service) error {
	privatePort, _ := strconv.Atoi(service.Origin.ExposedPort)
	publicPortString := strconv.Itoa(service.Port)
	acl := zk.WorldACL(zk.PermAll)
	basePath := r.path + "/" + service.Name
	if r.path == "/" {
		basePath = r.path + service.Name
	}
	exists, _, err := r.client.Exists(basePath)
	if err != nil {
		log.Error("zookeeper: error checking if exists", "error", err)
	} else {
		if !exists {
			_, err := r.client.Create(basePath, []byte{}, 0, acl)
			if err != nil {
				log.Error("zookeeper: failed to create base service node at path '"+basePath+"'", "error", err)
			}
		} // create base path for the service name if it missing
		zbody := &ZnodeBody{Name: service.Name, IP: service.IP, PublicPort: service.Port, PrivatePort: privatePort, Tags: service.Tags, Attrs: service.Attrs, ContainerID: service.Origin.ContainerHostname}
		body, err := json.Marshal(zbody)
		if err != nil {
			log.Error("zookeeper: failed to json encode service body", "error", err)
		} else {
			path := basePath + "/" + service.IP + ":" + publicPortString
			_, err = r.client.Create(path, body, 1, acl)
			if err != nil {
				log.Error("zookeeper: failed to register service at path '"+path+"'", "error", err)
			} // create service path error check
		} // json znode body creation check
	} // service path exists error check
	return err
}

func (r *Zookeeper) Ping() error {
	_, _, err := r.client.Exists("/")
	if err != nil {
		log.Error("zookeeper: error on ping check for Exists(/)", "error", err)
		return err
	}
	return nil
}

func (r *Zookeeper) Deregister(service *bridge.Service) error {
	basePath := r.path + "/" + service.Name
	if r.path == "/" {
		basePath = r.path + service.Name
	}
	publicPortString := strconv.Itoa(service.Port)
	servicePortPath := basePath + "/" + service.IP + ":" + publicPortString
	// Delete the service-port znode
	err := r.client.Delete(servicePortPath, -1) // -1 means latest version number
	if err != nil {
		log.Error("zookeeper: failed to deregister service port entry", "error", err)
	}
	// Check if all service-port znodes are removed.
	children, _, err := r.client.Children(basePath)
	if len(children) == 0 {
		// Delete the service name znode
		err := r.client.Delete(basePath, -1)
		if err != nil {
			log.Error("zookeeper: failed to delete service path", "error", err)
		}
	}
	return err
}

func (r *Zookeeper) Refresh(service *bridge.Service) error {
	return r.Register(service)
}

func (r *Zookeeper) Services() ([]*bridge.Service, error) {
	return []*bridge.Service{}, nil
}
