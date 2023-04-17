*** Settings ***
Library           String
Library           Process
Library           OperatingSystem

*** Keywords ***
#####
# Infra - Start / Stop Schema- and Data-server 
#####
Setup
    [Documentation]    Starts schema and data server. Waits for the dataserver to begin sync before returning
    [Arguments]    ${doBuild}    ${server-bin}    ${schema-server-config}    ${schema-server-process-alias}    ${schema-server-stderr}    ${data-server-config}    ${data-server-process-alias}    ${data-server-stderr}
    IF    ${doBuild} == $True
        ${result} =     Run Process    make     build
        Log Many	stdout: ${result.stdout}	stderr: ${result.stderr}
    END
    Start Process    ${server-bin}  -c     ${schema-server-config}    alias=${schema-server-process-alias}        stderr=${schema-server-stderr}
    Start Process    ${server-bin}  -c     ${data-server-config}    alias=${data-server-process-alias}    stderr=${data-server-stderr}
    WaitForOutput    ${data-server-stderr}    sync    180x    3s

Teardown
    [Documentation]    Stop all the started schema-server, data-server and client processes 
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
    [Arguments]    ${schema-server-process-alias}    ${data-server-process-alias} 
    Process Should Be Running    handle=${schema-server-process-alias}    error_message="schema-server failed"
    Process Should Be Running    handle=${data-server-process-alias}    error_message="data-server failed"