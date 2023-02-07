# SCHEMA-SERVER

## run the server

```shell
go build
./schema-server
```

## build client and run cmds

```shell
cd client
go build

# schema
./client schema get --name srl --version 22.11.1 --vendor Nokia --path /interface[name=ethernet-1/1]/subinterface
./client schema get --name srl --version 22.11.1 --vendor Nokia --path /acl/cpm-filter/ipv4-filter/entry/action/accept/rate-limit/system-cpu-policer
./client schema to-path --name srl --version 22.11.1 --vendor Nokia --cp interface,mgmt0,admin-state
./client schema expand --name srl --version 22.11.1 --vendor Nokia --path interface[name=ethernet-1/1]

# datastore
./client -a clab-distributed-data-server:56000 datastore get --ds srl1
## create a candidate datastore
./client -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate default
./client -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate temp
./client -a clab-distributed-data-server:56000 datastore get --ds srl1
# delete candidate "temp" datastore
./client -a clab-distributed-data-server:56000 datastore delete --ds srl1 --candidate temp
./client -a clab-distributed-data-server:56000 datastore get --ds srl1

# data
## state
./client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=*]/subinterface[index=0]/statistics/in-octets
./client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=*]/subinterface[index=0]/statistics/in-octets --candidate default
## configure
./client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/admin-state:::disable
./client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/description:::desc1
./client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/subinterface[index=0]/admin-state:::enable
./client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/subinterface[index=0]/description:::desc1
### get fom candidate
./client -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/admin-state
./client -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/description
./client -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/subinterface[index=0]/admin-state
./client -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/subinterface[index=0]/description
### get from main
./client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/admin-state
./client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/description
./client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/subinterface[index=0]/admin-state
./client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/subinterface[index=0]/description
# diff
./client -a clab-distributed-data-server:56000 data diff --ds srl1 --candidate default
### commit
./client -a clab-distributed-data-server:56000 datastore commit --ds srl1 --candidate default
./client -a clab-distributed-data-server:56000 datastore get --ds srl1
```
