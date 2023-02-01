./client/client datastore get --ds srl1
./client/client datastore create --ds srl1 --candidate default
./client/client datastore get --ds srl1

./client/client datastore get --ds srl2
./client/client datastore create --ds srl2 --candidate default
./client/client datastore get --ds srl2

date -Ins
for i in $(seq 1 1000);
do 
# date -Ins
./client/client data set --ds srl1 --candidate default  --update interface[name=ethernet-1/1]/admin-state:::enable \
                                                        --update interface[name=ethernet-1/1]/vlan-tagging:::true \
                                                        --update interface[name=ethernet-1/1]/description:::interface_desc$i \
                                                        --update interface[name=ethernet-1/1]/subinterface[index=$i]/admin-state:::enable \
                                                        --update interface[name=ethernet-1/1]/subinterface[index=$i]/type:::bridged \
                                                        --update interface[name=ethernet-1/1]/subinterface[index=$i]/description:::subinterface_desc$i \
                                                        --update interface[name=ethernet-1/1]/subinterface[index=$i]/vlan/encap/single-tagged/vlan-id:::$((i+1)) > /dev/null
#
# date -Ins
done

date -Ins
# ./client/client data diff --ds srl1 --candidate default > /dev/null
# date
# ./client/client datastore commit --ds srl1 --candidate default
# date
