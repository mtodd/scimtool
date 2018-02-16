# `ldap-bridged`

Provides an LDAP-to-SCIM bridge for [GitHub Business organizations](https://github.com/business) with SAML SSO configured and enabled.

This tool will mirror an LDAP Group's memberships (the identity provider) to the configured SCIM-enabled service provider.

The LDAP Directory is continuously monitored for changes in memberships which will trigger.

When a new member is added to the group, that member is looked up in LDAP, mapped to an equivalent SCIM representation, and then provisioned on the SCIM-enabled service provider.

When a member is provisioned in SCIM, we record the `ID` returned from the service provider and store it internally for future cross-references.

When a member is removed from the group, a similar flow occurs and then member is removed from the SCIM-enabled organization.

The tool will synchronize the IdP and the SP when starting up.

Finally, the tool provides a web interface to view the internal state of the bridge which may be helpful for troubleshooting. Access this view at http://localhost:4444/.

## Status

This is a very early prototype; consider this alpha software. Use at your own risk. No warantee is expressed or implied, etc. See license.

Currently [GitHub.com Business accounts](https://github.com/business) are the only SCIM-capable Service Providers supported but generalized SCIM support is planned.

Better configurable runtime behavior is anticipated such as:
- a configuration file format
- configurable attribute mapping
- one-to-many IdP-to-SP configurations
- IdP and SP adapters to support more options

Expect configuration to move either to a configuration file format or be made available as command line arguments and flags.

## Usage

``` shell
$ SCIM_ORG=$org SCIM_DRY=false ldap-bridged 
```

## Configuration

Configuration is currently handled via ENV variables:

### LDAP

- `LDAP_ADDR` the host and port of the LDAP directory to monitor (default: `localhost:389`)
- `LDAP_BIND` the Distinguished Name (DN) of the admin to bind the connection as
- `LDAP_PASS` the password of the admin that binds the connection
- `LDAP_BASE` the Base DN to search
- `LDAP_GROUP` the DN of the LDAP Group to monitor

### SCIM

- `SCIM_ORG` the name of the GitHub.com Business organization with SAML-enabled
- `SCIM_TOKEN` the authorization token (with `admin:org` scope) to manage the configured `SCIM_ORG`
- `SCIM_DRY` used to enable provisioning for the configured organization by setting to `false` (default: `true`)

### Bridge

- `DB` the path to the internal state database file (default: `bridge.db`)

## License

Copyright 2018 Matt Todd

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"), to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
