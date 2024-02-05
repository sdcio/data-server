*** Settings ***
Library           String
Library           Process
Library           OperatingSystem

*** Keywords ***
#####
# Infra - Start / Stop Schema- and Data-server 
#####

DeployLab
    [Documentation]    Deploys a containerlab topology
    [Arguments]       ${topology-file}    
    ${rc}   ${result} =   Run And Return Rc And Output
    ...    sudo containerlab deploy -t ${topology-file} -c
    Log         ${result}
    RETURN      ${rc}   ${result}

DestroyLab
    [Documentation]    Destroys a containerlab topology
    [Arguments]    ${topology-file}
    ${rc}   ${result} =   Run And Return Rc And Output
    ...  sudo containerlab des -t ${topology-file} -c  
    Log         ${result}   
    RETURN      ${rc}   ${result}

Setupcollocated
    [Documentation]    Starts a data-server with an embeded schema and cache stores.
    [Arguments]    ${doBuild}    ${data-server-bin}    ${data-server-config}    ${data-server-process-alias}    ${data-server-stderr}
    IF    ${doBuild} == $True
        ${result} =     Run Process    make     build
        Log Many	stdout: ${result.stdout}	stderr: ${result.stderr}
    END
    Start Process    ${data-server-bin}  -c     ${data-server-config}    alias=${data-server-process-alias}        stderr=${data-server-stderr}
    WaitForOutput    ${data-server-stderr}    ready...    10x    1s

Setup
    [Documentation]    Starts schema and data server. Waits for the dataserver to begin sync before returning
    [Arguments]    ${doBuild}    ${server-bin}    ${cache-bin}    ${schema-server-config}    ${schema-server-process-alias}    ${schema-server-stderr}    ${data-server-config}    ${data-server-process-alias}    ${data-server-stderr}    ${cache-server-config}    ${cache-server-process-alias}    ${cache-server-stderr}
    IF    ${doBuild} == $True
        ${result} =     Run Process    make     build
        Log Many	stdout: ${result.stdout}	stderr: ${result.stderr}
    END
    Start Process    ${server-bin}  -c     ${schema-server-config}    alias=${schema-server-process-alias}        stderr=${schema-server-stderr}
    Start Process    ${server-bin}  -c     ${data-server-config}    alias=${data-server-process-alias}    stderr=${data-server-stderr}
    Start Process    ${cache-bin}  -c     ${cache-server-config}    alias=${cache-server-process-alias}    stderr=${cache-server-stderr}
    WaitForOutput    ${data-server-stderr}    sync    3x    3s

Teardown
    [Documentation]    Stop the started data-server
    ${rc}   ${result} =   Run And Return Rc And Output
    ...  rm -rf ../../cached/caches  
    Log         ${result}   
    Terminate All Processes

# Infra Helper
WaitForOutput
    [Documentation]    Takes a file, pattern, retries and check_intervall. With this will open the file searching for the pattern in the given interval and return
    ...                on found. If not found witin retries x check_intervall time frame, will fail.
    [Arguments]    ${file}    ${pattern}    ${retries}    ${check_intervall}
    Wait Until Keyword Succeeds    ${retries}    ${check_intervall}    _CheckOutput    ${file}    ${pattern}

_CheckOutput
    [Documentation]    reads the given file and searches for the given pattern. Fails if not found. 
    [Arguments]    ${file}    ${pattern}
    ${ret} =	Grep File     ${file}    ${pattern}
    ${cnt}=    Get length    ${ret}
    IF    ${cnt} > 0
        RETURN
    ELSE
        Fail    Pattern (${pattern}) not found in file ${file}.    
    END    
    
CheckServerState
    [Documentation]    Check that schema-server and data-server are still running
    [Arguments]    ${schema-server-process-alias}    ${data-server-process-alias}    ${cache-server-process-alias} 
    Process Should Be Running    handle=${schema-server-process-alias}    error_message="schema-server failed"
    Process Should Be Running    handle=${data-server-process-alias}    error_message="data-server failed"
    Process Should Be Running    handle=${cache-server-process-alias}    error_message="cache-server failed"