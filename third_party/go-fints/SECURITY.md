# Security policy

## Reporting a vulnerability

Use GitHub's private vulnerability reporting for this repository. Do not open a
public issue containing an exploit, credential, raw FinTS message, or financial
data.

Include the affected commit or release, impact, reproduction using synthetic
data, and any suggested mitigation. Maintainers will acknowledge the report and
coordinate disclosure when possible.

If banking credentials or account data were exposed, contact the bank
immediately, revoke or change the affected credentials, and do not send them to
this project.

## Scope

Security reports may cover credential handling, transport validation, message
construction, C-ABI memory safety, and unintended payment execution. General
bank interoperability problems without a security impact belong in the issue
tracker after all sensitive information has been removed.
