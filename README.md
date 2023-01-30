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
./client datastore get --ds srl1
## create a candidate datastore
./client datastore create --ds srl1 --candidate default
./client datastore create --ds srl1 --candidate temp
./client datastore get --ds srl1
# delete candidate "temp" datastore
./client datastore delete --ds srl1 --candidate temp
./client datastore get --ds srl1

# data
## state
./client data get --ds srl1 --path interface[name=*]/subinterface[index=0]/statistics/in-octets
./client data get --ds srl1 --path interface[name=*]/subinterface[index=0]/statistics/in-octets --candidate default
## configure
./client data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/admin-state:::disable
./client data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/description:::desc1
### get fom candidate
./client data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/admin-state
./client data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/description
### get from main
./client data get --ds srl1 --path interface[name=ethernet-1/1]/admin-state
./client data get --ds srl1 --path interface[name=ethernet-1/1]/description
# diff
./client data diff --ds srl1 --candidate default
### commit
./client datastore commit --ds srl1 --candidate default
./client datastore get --ds srl1
```
