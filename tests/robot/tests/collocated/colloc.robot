*** Settings ***
Resource          ../../keywords/server.robot
Resource          ../../keywords/client.robot
Resource          ../../keywords/gnmic.robot
Library           OperatingSystem
Library           String
Library           Process
Suite Setup       server.DeployLab          ${topology-file}
Suite Teardown    server.DestroyLab          ${topology-file}

*** Variables ***
${data-server-bin}       ../../bin/data-server
${data-server-config}    ./tests/collocated/collocated_test.yaml
${client-bin}            ../../bin/datactl
${data-server-ip}        127.0.0.1
${data-server-port}      56000
${topology-file}         ./tests/collocated/lab/collocated.clab.yaml

# TARGET
${schema-name}           srl
${schema-version}        23.10.1
${schema-vendor}         Nokia
${candidate-name}        default

${datastore1}            srl1
${datastore2}            srl2
${target-file1}          ./tests/collocated/robot_srl1.json
${target-file2}          ./tests/collocated/robot_srl2.json
${target-sync-file}      ./tests/collocated/sync.json

${router}                ""
${gnmic_flags}           ""


# internal vars
${data-server-process-alias}    dsa
${data-server-stderr}          /tmp/ds-out


*** Test Cases ***

Create and delete a Datastore
   [Tags]    robot:continue-on-failure

   server.Setupcollocated    True     ${data-server-bin}     ${data-server-config}    ${data-server-process-alias}    ${data-server-stderr}

   # create first datastore
   ${rc}  ${result} =  client.CreateDataStoreTarget    ${datastore1}     ${target-file1}     ${target-sync-file}     ${schema-name}     ${schema-vendor}     ${schema-version}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    ${datastore1}
   
   # create second datastore
   ${rc}  ${result} =  client.CreateDataStoreTarget    ${datastore2}     ${target-file2}     ${target-sync-file}     ${schema-name}     ${schema-vendor}     ${schema-version}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Sleep   30s
   
   ${rc}  ${result} =  client.GetDataStore       ${datastore1}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain  ${result}   name: "${datastore1}"
   Should Contain  ${result}   status: CONNECTED

   ${rc}  ${result} =  client.DeleteDatastore    ${datastore1}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   ${rc}  ${result} =  client.GetDataStore       ${datastore1}
   Log     ${result}
   Should Not Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    unknown datastore ${datastore1}

   # ${rc}  ${result} =  server.DestroyLab          ${topology-file}
   # Log     ${result}
   # Should Be Equal As Integers    ${rc}    0

   server.Teardown

Create and delete a Datastore and candidate
   [Tags]    robot:continue-on-failure

  
   server.Setupcollocated    True     ${data-server-bin}     ${data-server-config}    ${data-server-process-alias}    ${data-server-stderr}  
   
   # create first datastore
   ${rc}  ${result} =  client.CreateDataStoreTarget    ${datastore1}     ${target-file1}     ${target-sync-file}     ${schema-name}     ${schema-vendor}     ${schema-version}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    ${datastore1}

   # create second datastore
   ${rc}  ${result} =  client.CreateDataStoreTarget    ${datastore2}     ${target-file2}     ${target-sync-file}     ${schema-name}     ${schema-vendor}     ${schema-version}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   
   Sleep  30s

   ${rc}  ${result} =  client.GetDataStore       ${datastore1}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain  ${result}   name: "${datastore1}"
   Should Contain  ${result}   status: CONNECTED
   
   ${rc}  ${result} =  client.CreateCandidate    ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   ${rc}  ${result} =  client.GetCandidate       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    ${datastore1}
   Should Contain    ${result}    ${candidate-name}

   ${rc}  ${result} =  client.DeleteCandidate       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    ${datastore1}

   ${rc}  ${result} =  client.GetCandidate       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    ${datastore1}
   Should Not Contain    ${result}    ${candidate-name}

   server.Teardown

Configure Router gNMI - Set / Delete Leaf
   [Tags]    robot:continue-on-failure
   
   server.Setupcollocated    True     ${data-server-bin}     ${data-server-config}    ${data-server-process-alias}    ${data-server-stderr}
   
   ${rc}   ${result} =   Run And Return Rc And Output
    ...        schemac -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} schema list
   Log     ${result}

   # delete interface config
   ${rc}  ${result} =  gnmic.Set   clab-collocated-srl1  --skip-verify -u admin -p NokiaSrl1! --delete /interface[name=ethernet-1/1]
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   ${rc}  ${result} =  client.CreateDataStoreTarget    ${datastore1}     ${target-file1}     ${target-sync-file}     ${schema-name}     ${schema-vendor}     ${schema-version}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Sleep   30s

   ${rc}   ${result} =  client.GetDataStore       ${datastore1}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain  ${result}   name: "${datastore1}"
   Should Contain  ${result}   status: CONNECTED

   # create candidate
   ${rc}  ${result} =  client.CreateCandidate    ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   ${rc}  ${result} =  client.GetCandidate       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    ${datastore1}
   Should Contain    ${result}    ${candidate-name}

   # set config into candidate
   ${rc}  ${result} =  client.Set       ${datastore1}   ${candidate-name}   --update /interface[name=ethernet-1/1]/description:::Desc1
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   # get config from candidate
   ${rc}  ${result} =  client.GetFromCandidate       ${datastore1}   ${candidate-name}   /interface[name=ethernet-1/1]/description
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    Desc1

   # run diff
   ${rc}  ${result} =  client.Diff       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    Desc1

   # commit
   ${rc}  ${result} =  client.Commit       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   # query fom intended store
   ${rc}  ${result} =  client.Get      ${datastore1}   --intended --path /interface[name=ethernet-1/1]/description --priority -1
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    Desc1

   # query fom router
   ${rc}  ${result} =  gnmic.Get   clab-collocated-srl1  --skip-verify -e JSON_IETF -u admin -p NokiaSrl1! --format flat --path /interface[name=ethernet-1/1]/description
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    Desc1

   # DELETE
   # create candidate
   ${rc}  ${result} =  client.CreateCandidate    ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   ${rc}  ${result} =  client.GetCandidate       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    ${datastore1}
   Should Contain    ${result}    ${candidate-name}

   # set config into candidate
   ${rc}  ${result} =  client.Set       ${datastore1}   ${candidate-name}   --delete /interface[name=ethernet-1/1]/description
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   # get config from candidate
   ${rc}  ${result} =  client.GetFromCandidate       ${datastore1}   ${candidate-name}   /interface[name=ethernet-1/1]/description
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Not Contain    ${result}    Desc1

   # run diff
   ${rc}  ${result} =  client.Diff       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    Desc1

   # commit
   ${rc}  ${result} =  client.Commit       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   # query fom intended store
   ${rc}  ${result} =  client.Get      ${datastore1}   --intended --path /interface[name=ethernet-1/1]/description --priority -1
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Not Contain    ${result}    Desc1

   # query fom router
   ${rc}  ${result} =  gnmic.Get   clab-collocated-srl1  --skip-verify -e JSON_IETF -u admin -p NokiaSrl1! --format flat --path /interface[name=ethernet-1/1]/description
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Not Contain    ${result}    Desc1

   server.Teardown


Configure Router gNMI - Create / Delete List item
   [Tags]    robot:continue-on-failure
   
   server.Setupcollocated    True     ${data-server-bin}     ${data-server-config}    ${data-server-process-alias}    ${data-server-stderr}
   
   ${rc}   ${result} =   Run And Return Rc And Output
    ...        schemac -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} schema list
   Log     ${result}
   
   # delete interface config in case there are some remains
   ${rc}  ${result} =  gnmic.Set   clab-collocated-srl1  --skip-verify -u admin -p NokiaSrl1! --delete /interface[name=ethernet-1/1]
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   ${rc}  ${result} =  client.CreateDataStore    ${datastore1}     ${target-file1}     ${target-sync-file}     ${schema-name}     ${schema-vendor}     ${schema-version}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Sleep   30s

   ${rc}   ${result} =  client.GetDataStore       ${datastore1}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain  ${result}   name: "${datastore1}"
   Should Contain  ${result}   status: CONNECTED

   # create candidate
   ${rc}  ${result} =  client.CreateCandidate    ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   ${rc}  ${result} =  client.GetCandidate       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    ${datastore1}
   Should Contain    ${result}    ${candidate-name}

   # set config into candidate
   ${rc}  ${result} =  client.Set       
   ...    ${datastore1}   ${candidate-name}  --update /interface[name=ethernet-1/1]/description:::Desc1 --update /interface[name=ethernet-1/1]/subinterface[index=0]/admin-state:::enable --update /interface[name=ethernet-1/1]/subinterface[index=0]/description:::DescSub0
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   # get config from candidate
   ${rc}  ${result} =  client.GetFromCandidate       ${datastore1}   ${candidate-name}   /interface[name=ethernet-1/1]/description
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    Desc1
   
   # get config from candidate
   ${rc}  ${result} =  client.GetFromCandidate       ${datastore1}   ${candidate-name}   /interface[name=ethernet-1/1]/subinterface[index=0]
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    DescSub0

   # run diff
   ${rc}  ${result} =  client.Diff       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    Desc1
   Should Contain    ${result}    DescSub0

   # commit
   ${rc}  ${result} =  client.Commit       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   # query from router
   ${rc}  ${result} =  gnmic.Get   clab-collocated-srl1  --skip-verify -e ASCII -u admin -p NokiaSrl1! --format flat --path /interface[name=ethernet-1/1]/description
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    Desc1

   # query from router
   ${rc}  ${result} =  gnmic.Get   clab-collocated-srl1  --skip-verify -e ASCII -u admin -p NokiaSrl1! --format flat --path /interface[name=ethernet-1/1]/subinterface[index=0]/description
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    DescSub0
   
   # DELETE
   # create candidate
   ${rc}  ${result} =  client.CreateCandidate    ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   ${rc}  ${result} =  client.GetCandidate       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    ${datastore1}
   Should Contain    ${result}    ${candidate-name}

   # set config into candidate
   ${rc}  ${result} =  client.Set       
   ...    ${datastore1}   ${candidate-name}  --delete /interface[name=ethernet-1/1]/description --delete /interface[name=ethernet-1/1]/subinterface[index=0]/description
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   # get config from candidate
   ${rc}  ${result} =  client.GetFromCandidate       ${datastore1}   ${candidate-name}   /interface[name=ethernet-1/1]
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Not Contain    ${result}    Desc1
   Should Not Contain    ${result}    DescSub0
   
   # run diff
   ${rc}  ${result} =  client.Diff       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Contain    ${result}    Desc1
   Should Contain    ${result}    DescSub0

   # commit
   ${rc}  ${result} =  client.Commit       ${datastore1}   ${candidate-name}
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0

   # query fom intended store
   ${rc}  ${result} =  client.Get      ${datastore1}   --intended --path /interface[name=ethernet-1/1]/description --priority -1
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Not Contain    ${result}    Desc1

   # query from router
   ${rc}  ${result} =  gnmic.Get   clab-collocated-srl1  --skip-verify -e ASCII -u admin -p NokiaSrl1! --format flat --path /interface[name=ethernet-1/1]/description
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Not Contain    ${result}    Desc1

   # query fom intended store
   ${rc}  ${result} =  client.Get      ${datastore1}   --intended --path /interface[name=ethernet-1/1]/subinterface[index=0]/description --priority -1
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Not Contain    ${result}    DescSub0

   # query from router
   ${rc}  ${result} =  gnmic.Get   clab-collocated-srl1  --skip-verify -e ASCII -u admin -p NokiaSrl1! --format flat --path /interface[name=ethernet-1/1]/subinterface[index=0]/description
   Log     ${result}
   Should Be Equal As Integers    ${rc}    0
   Should Not Contain    ${result}    DescSub0

   server.Teardown
