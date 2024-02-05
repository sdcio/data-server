*** Settings ***
Documentation     The Library allows running gnmic commands
Library           String
Library           Process
Library           OperatingSystem

*** Keywords ***
Capabilities
    [Documentation]   run capabilities using gNMIc
    [Arguments]    ${router}  ${gnmic_flags}   ${path}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    gnmic -a ${router} cap ${gnmic_flags} --log
    Log    ${result}
    RETURN  ${rc}  ${result}

Get
    [Documentation]  run Get using gNMIc
    [Arguments]    ${router}  ${gnmic_flags}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    gnmic -a ${router} get ${gnmic_flags} --log
    Log    ${result}
    RETURN  ${rc}  ${result}

Set
    [Documentation]   run Set using gNMIc
    [Arguments]    ${router}  ${gnmic_flags}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    gnmic -a ${router} set ${gnmic_flags} --log
    Log    ${result}
    RETURN  ${rc}  ${result}

SubscribeOnce
    [Documentation]  run Subscribe mode ONCE using gNMIc
    [Arguments]    ${router}  ${gnmic_flags}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    gnmic -a ${router} sub ${gnmic_flags} --log
    Log    ${result}
    RETURN  ${rc}  ${result}

Subscribe
    [Documentation]  run Subscribe using gNMIc
    [Arguments]    ${router}  ${gnmic_flags}
    ${rc}   ${result} =    Run And Return Rc And Output    
    ...    gnmic -a ${router} sub ${gnmic_flags} --log
    Start Process    gnmic -a ${router} sub ${gnmic_flags}    alias=${gnmic-process-alias}        stderr=${gnmic-stderr}
    WaitForOutput    ${gnmic-stderr}    "subscribing"