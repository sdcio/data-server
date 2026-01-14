# SDC data-server

![sdc logo](https://docs.sdcio.dev/assets/logos/SDC-transparent-withname-100x133.png)

This repository is part of Schema Driven Configuration (SDC)

The paradigm of schema-driven API approaches is gaining increasing popularity as it facilitates programmatic interaction with systems by both machines and humans. While OpenAPI schema stands out as a widely embraced system, there are other notable schema approaches like YANG, among others. This project endeavors to empower users with a declarative and idempotent method for seamless interaction with API systems, providing a robust foundation for effective system configuration."

The data-server component serves as a versatile intermediary, connecting the config-server, schema-server, cache, and xNF/Device in a stateless design for scalability. It features a North-bound API for both imperative and declarative interactions and supports various South-bound protocols. With dedicated DataStores per target, flexible synchronization options, candidate-based interactions, and the ability to connect multiple data servers per device, it provides a resilient and adaptable foundation for managing and synchronizing data in dynamic system environments.

## build

```shell
make build
```

## run the server

```shell
./bin/data-server
```

## run the client

```shell
bin/datactl -a clab-distributed-data-server:56000 datastore get --ds srl1


## create a candidate datastore
bin/datactl -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate default
bin/datactl -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate temp
bin/datactl -a clab-distributed-data-server:56000 datastore get --ds srl1
# delete candidate "temp" datastore
bin/datactl -a clab-distributed-data-server:56000 datastore delete --ds srl1 --candidate temp
bin/datactl -a clab-distributed-data-server:56000 datastore get --ds srl1

# data
## state
bin/datactl -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=*]/subinterface[index=0]/statistics/in-octets
bin/datactl -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=*]/subinterface[index=0]/statistics/in-octets --candidate default
## configure
bin/datactl -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/admin-state:::disable
bin/datactl -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/description:::desc1
bin/datactl -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/subinterface[index=0]/admin-state:::enable
bin/datactl -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default --update interface[name=ethernet-1/1]/subinterface[index=0]/description:::desc1
### get fom candidate
bin/datactl -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/admin-state
bin/datactl -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/description
bin/datactl -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/subinterface[index=0]/admin-state
bin/datactl -a clab-distributed-data-server:56000 data get --ds srl1 --candidate default --path interface[name=ethernet-1/1]/subinterface[index=0]/description
### get from main
bin/datactl -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/admin-state
bin/datactl -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/description
bin/datactl -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/subinterface[index=0]/admin-state
bin/datactl -a clab-distributed-data-server:56000 data get --ds srl1 --path interface[name=ethernet-1/1]/subinterface[index=0]/description
# diff
bin/datactl -a clab-distributed-data-server:56000 data diff --ds srl1 --candidate default
### commit
bin/datactl -a clab-distributed-data-server:56000 datastore commit --ds srl1 --candidate default
bin/datactl -a clab-distributed-data-server:56000 datastore get --ds srl1
```

### SROS

```shell
bin/datactl -a clab-distributed-data-server:56000 datastore create --ds sr1 --candidate default
bin/datactl -a clab-distributed-data-server:56000 datastore get --ds sr1
bin/datactl -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default --update /configure/system/name:::sr123
bin/datactl -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default --update /configure/service/vprn[service-name=vprn1]/customer:::1
bin/datactl -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default --update /configure/service/vprn[service-name=vprn1]/service-id:::100
bin/datactl -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default --update /configure/service/vprn[service-name=vprn1]/admin-state:::enable
##
# bin/datactl -a clab-distributed-data-server:56000 data get --ds sr1 --path /configure/system/name
bin/datactl -a clab-distributed-data-server:56000 data get --ds sr1 --candidate default --path /configure/system/name
bin/datactl -a clab-distributed-data-server:56000 data get --ds sr1 --candidate default --path /configure/service/vprn[service-name=vprn1]
bin/datactl -a clab-distributed-data-server:56000 datastore commit --ds sr1 --candidate default
```

```shell
bin/datactl -a clab-distributed-data-server:56000 datastore create --ds sr1 --candidate default

bin/datactl -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default  \
             --update-path /configure/router[router-name=Base]/interface[interface-name=system] \
             --update-file lab/common/configs/sros_interface_base.json
bin/datactl -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default  \
             --update-path configure/service/vprn[service-name=vprn1] \
             --update-file lab/common/configs/sros_vprn.json
bin/datactl -a clab-distributed-data-server:56000 data set --ds sr1 --candidate default  \
             --update-path /configure/router[router-name=Base]/interface \
             --update-file lab/common/configs/sros_interfaces_base.json
bin/datactl -a clab-distributed-data-server:56000 datastore commit --ds sr1 --candidate default
```

### SRL JSON VALUE

```shell
bin/datactl -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate default

bin/datactl -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default  \
             --update-path / \
             --update-file lab/common/configs/srl_interface.json
bin/datactl -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default \
             --update-path / \
             --update-file lab/common/configs/srl_interfaces.json

bin/datactl -a clab-distributed-data-server:56000 datastore commit --ds srl1 --candidate default
```

### leafref exp1: leafref as key

```shell
bin/datactl -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate default
bin/datactl -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default  \
             --update /system/gnmi-server/network-instance[name=default]/admin-state:::enable
bin/datactl -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default  \
             --update /system/gnmi-server/network-instance[name=default]/tls-profile:::clab-profile
bin/datactl -a clab-distributed-data-server:56000 datastore commit --ds srl1 --candidate default
```

### leafref exp2: leafref as leaf (not key)

```shell
# create candidate
bin/datactl -a clab-distributed-data-server:56000 datastore create --ds srl1 --candidate default

bin/datactl -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default  \
             --update /system/gnmi-server/network-instance[name=mgmt]/admin-state:::enable
bin/datactl -a clab-distributed-data-server:56000 data set --ds srl1 --candidate default  \
             --update /system/gnmi-server/network-instance[name=mgmt]/tls-profile:::dummy-profile
# commit
bin/datactl -a clab-distributed-data-server:56000 datastore commit --ds srl1 --candidate default
```

## Join us

Have questions, ideas, bug reports or just want to chat? Come join [our discord server](https://discord.com/channels/1240272304294985800/1311031796372344894).

## License and Code of Conduct

Code is under the [Apache License 2.0](LICENSE), documentation is [CC BY 4.0](LICENSE-documentation).

The SDC project is following the [CNCF Code of Conduct](https://github.com/cncf/foundation/blob/main/code-of-conduct.md). More information and links about the CNCF Code of Conduct are [here](code-of-conduct.md).
