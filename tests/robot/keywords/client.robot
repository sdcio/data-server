*** Settings ***
Documentation     The Library relies on the following variables being available on execution
...               CLIENT-BIN: Pointing to the client binary
...               DATA-SERVER-IP: IP of data server
...               DATA-SERVER-PORT: TCP port of data server
...               SCHEMA-SERVER-IP: IP of schema server
...               SCHEMA-SERVER-PORT: TCP port of schema server
Library           String
Library           Process
Library           OperatingSystem


*** Keywords ***
ListDataStores
    [Documentation]    List datastores in a data-server
    ${rc}   ${result} =   Run And Return Rc And Output
    ...        ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} datastore list
    RETURN  ${rc}  ${result}

GetDataStore 
    [Documentation]    Get a target from a data-server
    [Arguments]    ${datastore}
    ${rc}   ${result} =    Run And Return Rc And Output
    ...    ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} datastore get --ds ${datastore}
    RETURN  ${rc}  ${result}

CreateDataStore 
    [Documentation]    Create a target in a data-server
    [Arguments]    ${datastore}   ${target-definition-file}    ${sync-file}  ${schema-name}     ${schema-vendor}     ${schema-version}
    ${rc}   ${result} =     Run And Return Rc And Output
    ...    ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} datastore create --ds ${datastore} --target ${target-definition-file} --sync ${target-sync-file} --name ${schema-name} --vendor ${schema-vendor} --version ${schema-version}
    RETURN  ${rc}  ${result}

DeleteDatastore 
    [Documentation]    Delete a target from a data-server
    [Arguments]    ${datastore}
    ${rc}   ${result} =   Run And Return Rc And Output
    ...    ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} datastore delete --ds ${datastore}
    RETURN  ${rc}  ${result}

CreateCandidate
    [Documentation]    Create a new named Candidate in the given Datastore
    [Arguments]    ${datastore}    ${candidate}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} datastore create --ds ${datastore} --candidate ${candidate}
    RETURN  ${rc}  ${result}

GetCandidate
    [Documentation]    Create a new named Candidate in the given Datastore
    [Arguments]    ${datastore}    ${candidate}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} datastore get --ds ${datastore} --candidate ${candidate}
    RETURN  ${rc}  ${result}

DeleteCandidate
    [Documentation]    Delete a named Candidate in the given Datastore
    [Arguments]    ${datastore}    ${candidate}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} datastore delete --ds ${datastore} --candidate ${candidate}
    RETURN  ${rc}  ${result}

Diff
    [Documentation]    Performs a diff on a datastore and a candidate
    [Arguments]    ${datastore}    ${candidate}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} data diff --ds ${datastore} --candidate ${candidate}
    RETURN  ${rc}  ${result}

Commit
    [Documentation]    Performs a commit on the given datastore/candidate and returns the Process Result object https://robotframework.org/robotframework/latest/libraries/Process.html#Result%20object
    [Arguments]    ${datastore}    ${candidate}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} datastore commit --ds ${datastore} --candidate ${candidate}
    RETURN  ${rc}  ${result}

Get
    [Documentation]    Get a path from the datastore
    [Arguments]    ${datastore}    ${ds-get-flags}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} data get --ds ${datastore} ${ds-get-flags}
    Log    ${result}
    RETURN  ${rc}  ${result}

GetFromCandidate
    [Documentation]    Get a path from the datastore candidate
    [Arguments]    ${datastore}    ${candidate}    ${path}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} data get --ds ${datastore} --candidate ${candidate} --path ${path}
    Log    ${result}
    RETURN  ${rc}  ${result}

Set
    [Documentation]    Applies to the candidate of the given datastore the provided update
    [Arguments]    ${datastore}    ${candidate}    ${set-flags}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    ${CLIENT-BIN} -a ${DATA-SERVER-IP}:${DATA-SERVER-PORT} data set --ds ${datastore} --candidate ${candidate} ${set-flags}
    Log    ${result}
    RETURN  ${rc}  ${result}

GetSchema
    [Documentation]    Retrieve the schema element described by name (plattform name), version and vendor under the given path.
    [Arguments]    ${name}    ${version}    ${vendor}    ${path}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    ${CLIENT-BIN} -a ${SCHEMA-SERVER-IP}:${SCHEMA-SERVER-PORT} schema get --name ${name} --version ${version} --vendor ${vendor} --path ${path}    
    RETURN  ${rc}  ${result}

# Helper
ExtractResponse
    [Documentation]    Takes the output of the client binary and returns just the response part, stripping the request
    [Arguments]    ${output}
    @{split} =	Split String	${output}    response:
    RETURN    ${split}[1]

LogMustStatements
    [Documentation]    Takes vendor, name, version and a path, retrieves the schema for the given path, extracts the returned must_statements and logs them.
    [Arguments]    ${name}    ${version}    ${vendor}    ${path}
    ${schema} =     GetSchema     ${name}    ${version}    ${vendor}    ${path}
    ${msts} =    _ExtractMustStatements    ${schema.stdout}
    FOR    ${item}    IN    @{msts}
        Log    ${item}
    END

LogLeafRefStatements
    [Documentation]    Takes vendor, name, version and a path, retrieves the schema for the given path, extracts the returned leafref_statements and logs them.
    [Arguments]    ${name}    ${version}    ${vendor}    ${path}
    ${schema} =     GetSchema     ${name}    ${version}    ${vendor}    ${path}
    ${lref} =    _ExtractLeafRefStatements    ${schema.stdout}
    FOR    ${item}    IN    @{lref}
        Log    ${item}
    END

_ExtractMustStatements
    [Documentation]    Takes a GetSchema response and extracts the must_statements of the response. Returns an array with all the must_statements as a string array.
    [Arguments]    ${input}
    ${matches} =	Get Regexp Matches	${input}    must_statements:\\s*\{[\\s\\S]*?\}    flags=MULTILINE | IGNORECASE
    RETURN    ${matches}

_ExtractLeafRefStatements
    [Documentation]    Takes a GetSchema response and extracts the leafref_statements of the response.
    [Arguments]    ${input}
    ${matches} =	Get Regexp Matches	${input}    leafref:\\s*".*"    flags=IGNORECASE
    RETURN    ${matches}