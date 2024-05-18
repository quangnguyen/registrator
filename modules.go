package main

import (
	_ "github.com/quangnguyen/registrator/consul"
	_ "github.com/quangnguyen/registrator/consulkv"
	_ "github.com/quangnguyen/registrator/etcd"
	_ "github.com/quangnguyen/registrator/skydns2"
	_ "github.com/quangnguyen/registrator/zookeeper"
)
