# smbwatch

recursively retries all filenames from an smbshare and persists to a sqlite db.

## Installation and Usage

Clone and build (or downloaded a release):

    $ git clone
    $ go build
    
Usage:

    $ ./smbwatch -h
    Usage of ./smbwatch:
      -dbname string
            sqlite filename (default "sqlite.db")
      -debug
            debug mode
      -ldapDn string
            ldap distinguished name
      -ldapFilter string
            ldap filter to search for shares (default "(OperatingSystem=*server*)")
      -ldapServer string
            ldap server to get smb server list
      -maxdepth int
            max recursion depth when retrieving files (default 3)
      -pass string
            NTLM pass
      -server string
            smb server
      -user string
            NTLM user
      -worker int
            amount of parallel worker (default 8)
            
To enumerate a single server use the `-server` flag. To get a list of servers from
ldap and enumerate all of them, pass the `-ldap*` arguments.
    
Example:

    smbwatch -user A123456 -pass A123456 -ldapServer ldaps://foo.bar.internal.com:636 -ldapDn CN=A123456,OU=Users,OU=DE,DC=foo,DC=bar,DC=internal,DC=com
