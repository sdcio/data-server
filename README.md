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
./bin/client schema get --name srl --version 22.11.1 --vendor Nokia --path /interface[name=ethernet-1/1]/subinterface
./bin/client schema get --name srl --version 22.11.1 --vendor Nokia --path /acl/cpm-filter/ipv4-filter/entry/action/accept/rate-limit/system-cpu-policer
./bin/client schema to-path --name srl --version 22.11.1 --vendor Nokia --cp interface,mgmt0,admin-state
./bin/client schema expand --name srl --version 22.11.1 --vendor Nokia --path interface[name=ethernet-1/1]

# datastore
./bin/client -a clab-distributed-data-server:56000 datastore get --ds srl1
## create a candidate datastore
./bin/client -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate default
./bin/client -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate temp
./bin/client -a clab-distributed-data-server:56000 datastore get --ds srl1
# delete candidate "temp" datastore
./bin/client -a clab-distributed-data-server:56000 datastore delete --ds srl1 --candidate temp
./bin/client -a clab-distributed-data-server:56000 datastore get --ds srl1

# data
## state
./bin/client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=*]/subinterface[index=0]/statistics/in-octets
./bin/client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=*]/subinterface[index=0]/statistics/in-octets --candidate default
## configure
./bin/client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/admin-state:::disable
./bin/client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/description:::desc1
./bin/client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/subinterface[index=0]/admin-state:::enable
./bin/client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/subinterface[index=0]/description:::desc1
### get fom candidate
./bin/client -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/admin-state
./bin/client -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/description
./bin/client -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/subinterface[index=0]/admin-state
./bin/client -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/subinterface[index=0]/description
### get from main
./bin/client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/admin-state
./bin/client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/description
./bin/client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/subinterface[index=0]/admin-state
./bin/client -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/subinterface[index=0]/description
# diff
./bin/client -a clab-distributed-data-server:56000 data diff --ds srl1 --candidate default
### commit
./bin/client -a clab-distributed-data-server:56000 datastore commit --ds srl1 --candidate default
./bin/client -a clab-distributed-data-server:56000 datastore get --ds srl1
```

### SROS

```shell
./bin/client -a clab-distributed-data-server:56000 datastore create --ds sr1 --candidate default
./bin/client -a clab-distributed-data-server:56000 datastore get --ds sr1 
./bin/client -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default --update /configure/system/name:::sr123 
./bin/client -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default --update /configure/service/vprn[service-name=vprn1]/customer:::1
./bin/client -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default --update /configure/service/vprn[service-name=vprn1]/service-id:::100
./bin/client -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default --update /configure/service/vprn[service-name=vprn1]/admin-state:::enable
##
# ./bin/client -a clab-distributed-data-server:56000 data get --ds sr1 --path /configure/system/name
./bin/client -a clab-distributed-data-server:56000 data get --ds sr1 --candidate default --path /configure/system/name 
./bin/client -a clab-distributed-data-server:56000 data get --ds sr1 --candidate default --path /configure/service/vprn[service-name=vprn1]
./bin/client -a clab-distributed-data-server:56000 datastore commit --ds sr1 --candidate default
```

```shell
./bin/client -a clab-distributed-data-server:56000 datastore create --ds sr1 --candidate default

./bin/client -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default  \
             --update-path /configure/router[router-name=Base]/interface[interface-name=system] \
             --update-file lab/common/configs/sros_interface_base.json
./bin/client -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default  \
             --update-path configure/service/vprn[service-name=vprn1] \
             --update-file lab/common/configs/sros_vprn.json
./bin/client -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default  \
             --update-path /configure/router[router-name=Base]/interface \
             --update-file lab/common/configs/sros_interfaces_base.json
./bin/client -a clab-distributed-data-server:56000 datastore commit --ds sr1 --candidate default
```

### SRL JSON VALUE

```shell
./bin/client -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate default

./bin/client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default  \
             --update-path / \
             --update-file lab/common/configs/srl_interface.json
./bin/client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default \
             --update-path / \
             --update-file lab/common/configs/srl_interfaces.json

./bin/client -a clab-distributed-data-server:56000 datastore commit --ds srl1 --candidate default
```

### leafref exp1: leafref as key

```shell
./bin/client -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate default
./bin/client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default  \
             --update /system/gnmi-server/network-instance[name=default]/admin-state:::enable
./bin/client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default  \
             --update /system/gnmi-server/network-instance[name=default]/tls-profile:::clab-profile
./bin/client -a clab-distributed-data-server:56000 datastore commit --ds srl1 --candidate default
```

### leafref exp2: leafref as leaf (not key)

```shell
# create candidate
./bin/client -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate default

./bin/client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default  \
             --update /system/gnmi-server/network-instance[name=mgmt]/admin-state:::enable
./bin/client -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default  \
             --update /system/gnmi-server/network-instance[name=mgmt]/tls-profile:::dummy-profile
# commit
./bin/client -a clab-distributed-data-server:56000 datastore commit --ds srl1 --candidate default
```
