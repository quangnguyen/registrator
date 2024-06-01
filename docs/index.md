# Registrator

Service registry bridge for Docker.

[![Docker Hub](https://img.shields.io/badge/docker-ready-blue.svg)](https://hub.docker.com/r/lazylab/registrator)


Registrator automatically registers and deregisters services for any Docker
container by inspecting containers as they come online. Registrator
supports pluggable service registries, which currently includes
[Consul](http://www.consul.io/), [etcd](https://github.com/coreos/etcd) and
[SkyDNS 2](https://github.com/skynetservices/skydns/).

## Getting Registrator

Get the latest release, master, or any version of Registrator via [Docker Hub](https://hub.docker.com/r/lazylab/registrator):

	$ docker pull docker.io/lazylab/registrator:latest

Latest tag always points to the latest release. There is also a `:master` tag
and version tags to pin to specific releases.

## Using Registrator

The quickest way to see Registrator in action is our
[Quickstart](user/quickstart.md) tutorial. Otherwise, jump to the [Run
Reference](user/run.md) in the User Guide. Typically, running Registrator
looks like this:

    $ docker run -d \
        --name=registrator \
        --net=host \
        --volume=/var/run/docker.sock:/tmp/docker.sock \
        docker.io/lazylab/registrator:latest \
        consul://localhost:8500

## Contributing

Pull requests are welcome! We recommend getting feedback before starting by
opening a [GitHub issue](https://github.com/quangnguyen/registrator/issues)

Also check out our Developer Guide on [Contributing Backends](dev/backends.md)
and [Staging Releases](dev/releases.md).

## Sponsors and Thanks

## License

MIT

<img src="https://ga-beacon.appspot.com/UA-58928488-2/registrator/readme?pixel" />
