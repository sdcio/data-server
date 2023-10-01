#!/bin/bash

target_file="intra/static_srl2_target.json"

bin/datactl datastore create --ds srl2 --target $target_file --sync intra/sync.json --name srl --vendor Nokia --version 23.3.2

bin/datactl datastore list

num_candidates=500
datastore="srl2"
for i in $(seq 1 $num_candidates); do
  echo $i
  candidate="cand"$i
  owner="owner"$i
  bin/datactl datastore create --ds $datastore --candidate $candidate --owner $owner --priority $i
  bin/datactl data set --ds $datastore --candidate $candidate --update "interface[name=ethernet-1/1]/description:::desc_set_by_data-server$i"
done

bin/datactl datastore list

for i in $(seq 1 $num_candidates); do
candidate="cand"$i
bin/datactl data diff --ds $datastore --candidate $candidate # watch memory
bin/datactl data get --ds $datastore --candidate $candidate --path "interface[name=ethernet-1/1]/description" --format flat
done

for i in $(seq 1 $num_candidates); do
candidate="cand"$i
#bin/datactl data diff --ds $datastore --candidate $candidate # watch memory
bin/datactl datastore commit --ds $datastore --candidate $candidate
done

sleep 10
bin/datactl datastore delete --ds $datastore