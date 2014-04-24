LDAP authentication
===================

## Goal

Authenticat user against LDAP directories

It will bind with the user's login/pasword and query attributs ("mail" for instance) in a pool of directory servers

The first OK wins.

If there's connection error, the server will be disabled and won't be checked again

## Usage

In the [security] section, set 
>  LDAP_AUTH = true

then for each LDAP source, set

> [LdapSource-someuniquename]
> name=canonicalName
> host=hostname-or-ip
> port=3268	# or regular LDAP port
> # the following settings depend highly how you've configured your AD
> basedn=dc=ACME,dc=COM
> MSADSAFORMAT=%s@ACME.COM
> filter=(&(objectClass=user)(sAMAccountName=%s))

### Limitation

Only tested on an MS 2008R2 DC, using global catalog (TCP/3268)

This MSAD is a mess.

The way how one checks the directory (CN, DN etc...) may be highly depending local custom configuration

### Todo
* Define a timeout per server
* Check servers marked as "Disabled" when they'll come back online
* Find a more flexible way to define filter/MSADSAFORMAT/Attributes etc... maybe text/template ?
* Check OpenLDAP server
* SSL support ?