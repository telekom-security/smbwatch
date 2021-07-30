# smbwatch

recursively retries all filenames from an smbshare and persists to a sqlite db

## usage

    $ git clone
    $ go build
    $ ./smbwatch -h
    Usage of ./smbwatch:
      -dbname string
            sqlite filename (default "sqlite.db")
      -maxdepth int
            max recursion depth when retrieving files (default 3)
      -pass string
            NTLM pass
      -server string
            smb server
      -user string
            NTLM user
    
