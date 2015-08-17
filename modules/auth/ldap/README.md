Gogs LDAP Authentication Module
===============================

## About

This authentication module attempts to authorize and authenticate a user
against an LDAP server. Like most LDAP authentication systems, this module does
this in two steps. First, it queries the LDAP server using a Bind DN and
searches for the user that is attempting to sign in. If the user is found, the
module attempts to bind to the server using the user's supplied credentials. If
this succeeds, the user has been authenticated, and his account information is
retrieved and passed to the Gogs login infrastructure.

## Usage

To use this module, add an LDAP authentication source via the Authentications
section in the admin panel. The fields should be set as follows:

* Authorization Name **(required)**
	* A name to assign to the new method of authorization.

* Host **(required)**
	* The address where the LDAP server can be reached.
	* Example: mydomain.com

* Port **(required)**
	* The port to use when connecting to the server.
	* Example: 636

* Enable TLS Encryption (optional)
	* Whether to use TLS when connecting to the LDAP server.

* Bind DN (optional)
	* The DN to bind to the LDAP server with when searching for the user.
	This may be left blank to perform an anonymous search.
	* Example: cn=Search,dc=mydomain,dc=com

* Bind Password (optional)
	* The password for the Bind DN specified above, if any.

* User Search Base **(required)**
	* The LDAP base at which user accounts will be searched for.
	* Example: ou=Users,dc=mydomain,dc=com

* User Filter **(required)**
	* An LDAP filter declaring how to find the user record that is attempting
	to authenticate. The '%s' matching parameter will be substituted with
	the user's username.
	* Example: (&(objectClass=posixAccount)(uid=%s))

* First name attribute (optional)
	* The attribute of the user's LDAP record containing the user's first
	name. This will be used to populate their account information.
	* Example: givenName

* Surname name attribute (optional)
	* The attribute of the user's LDAP record containing the user's surname
	This will be used to populate their account information.
	* Example: sn

* E-mail attribute **(required)**
	* The attribute of the user's LDAP record containing the user's email
	address. This will be used to populate their account information.
	* Example: mail
