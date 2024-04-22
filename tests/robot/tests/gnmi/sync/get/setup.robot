*** Keywords ***
SuiteSetup
    DeployLab    ${topology-file}
    SetupColocated    True    ${DATA-SERVER-BIN}    ${DATA-SERVER-CONFIG}    ${data-server-process-alias}    ${data-server-stderr}
    CheckServerState Colocated    ${data-server-process-alias}

SuiteTeardown
    Teardown
    DestroyLab    ${topology-file}