*** Variables ***

${ROOT}                           ${CURDIR}/../../../../../../
${LibraryPath}                    ${ROOT}tests/robot/keywords
${BinPath}                        ${ROOT}/bin
${TestsPath}                      ${ROOT}/tests


${DATA-SERVER-BIN}                  ${BinPath}/data-server
${SDCTL}                            sdctl

${DATA-SERVER-CONFIG}               ${TestsPath}/robot/tests/must/data-server.yaml

${DATA-SERVER-IP}                   127.0.0.1
${DATA-SERVER-PORT}                 56000

${SCHEMA-SERVER-IP}                 127.0.0.1
${SCHEMA-SERVER-PORT}               56000

${topology-file}                    ${TestsPath}/robot/tests/colocated/lab/colocated.clab.yaml

# TARGET
${srlinux1-name}                    srl1
${srlinux1-hostname}                clab-colocated-srl1
${srlinux1-candidate}               default
${srlinux1-schema-name}             srl
${srlinux1-schema-version}          23.10.1
${srlinux1-schema-vendor}           Nokia
${srlinux1-target-def}              ${CURDIR}/../../srl1_target.json
${srlinux1-sync-def-01}                ${CURDIR}/sync-01.json
${srlinux1-sync-def-02}                ${CURDIR}/sync-02.json
${srlinux1-sync-def-03}                ${CURDIR}/sync-03.json

${owner}                            test
${priority}                         100

${srl-user}                         admin
${srl-password}                     NokiaSrl1!

# internal vars
${data-server-process-alias}        dsa
${data-server-stderr}               /tmp/ds-out

${interface-mgmt0-description-path}       /interface[name=mgmt0]/description
${interface-mgmt0-description-value}      my mgmt0 description

${interface-eth1-1-description-path}       /interface[name=ethernet-1/1]/description
${interface-eth1-1-description-value}      my ethernet-1/1 description