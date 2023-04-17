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
CreateCandidate
    [Documentation]    Create a new named Candidate in the given Datastore
    [Arguments]    ${datastore}    ${candidate}
    ${result} =    Run Process    ${CLIENT-BIN}     -a    ${DATA-SERVER-IP}:${DATA-SERVER-PORT}    datastore    create    --ds    ${datastore}    --candidate    ${candidate}
    RETURN    ${result}

DeleteCandidate
    [Documentation]    Delete a named Candidate in the given Datastore
    [Arguments]    ${datastore}    ${candidate}
    ${result} =    Run Process    ${CLIENT-BIN}     -a    ${DATA-SERVER-IP}:${DATA-SERVER-PORT}    datastore    delete    --ds    ${datastore}    --candidate    ${candidate}
    RETURN    ${result}

Commit
    [Documentation]    Performs a commit on the given datastore/candidate and returns the Process Result object https://robotframework.org/robotframework/latest/libraries/Process.html#Result%20object
    [Arguments]    ${datastore}    ${candidate}
    ${result} =    Run Process    ${CLIENT-BIN}     -a    ${DATA-SERVER-IP}:${DATA-SERVER-PORT}    datastore    commit    --ds    ${datastore}    --candidate    ${candidate}
    RETURN    ${result}

Set
    [Documentation]    Applies to the candidate of the given datastore the provided update
    [Arguments]    ${datastore}    ${candidate}    ${update}
    ${result} =    Run Process    ${CLIENT-BIN}     -a    ${DATA-SERVER-IP}:${DATA-SERVER-PORT}    data    set    --ds    ${datastore}    --candidate    ${candidate}    --update    ${update}
    Log    ${result.stdout}
    Log    ${result.stderr}
    RETURN    ${result}

GetSchema
    [Documentation]    Retrieve the schema element described by name (plattform name), version and vendor under the given path.
    [Arguments]    ${name}    ${version}    ${vendor}    ${path}
    ${result} =    Run Process    ${CLIENT-BIN}    -a    ${SCHEMA-SERVER-IP}:${SCHEMA-SERVER-PORT}    schema    get    --name    ${name}    --version    ${version}    --vendor    ${vendor}    --path    ${path}    
    RETURN    ${result}

GetDatastore
    [Documentation]   Performa get on the given Datastore
    [Arguments]    ${datastore}
    ${result} =    Run Process    ${CLIENT-BIN}     -a    ${DATA-SERVER-IP}:${DATA-SERVER-PORT}    datastore    get    --ds    ${datastore}
    RETURN    ${result}


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

_ExtractMustStatements
    [Documentation]    Takes a GetSchema response and extracts the must_statements of the response. Returns an array with all the must_statements as a string array.
    [Arguments]    ${input}
    ${matches} =	Get Regexp Matches	${input}    must_statements:\\s*\{[\\s\\S]*?\}    flags=MULTILINE | IGNORECASE
    RETURN    ${matches}