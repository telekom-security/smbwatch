package main

import (
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/hirochachacha/go-smb2"
)

var MaxDepth int

type ShareFile struct {
	ShareName string
	File      fs.FileInfo
	Folder    string
}

func smbSession(server, user, password string) (net.Conn, *smb2.Session, error) {
	conn, err := net.Dial("tcp", fmt.Sprintf("%v:445", server))
	if err != nil {
		return nil, nil, err
	}

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     user,
			Password: password,
		},
	}

	s, err := d.Dial(conn)
	if err != nil {
		return conn, nil, err
	}

	return conn, s, nil
}

func smbGetShareFiles(smbShare *smb2.Share, folder string) ([]ShareFile, error) {
	var sf []ShareFile

	if strings.Count(folder, `\`) > MaxDepth {
		return sf, fmt.Errorf("max depth of %v reached", MaxDepth)
	}

	f, err := smbShare.ReadDir(folder)
	if err != nil {
		return sf, fmt.Errorf("could not open folder %v: %v", folder, err)
	}

	for _, file := range f {

		if file.IsDir() {
			path := filepath.Join(folder, file.Name())
			path = strings.ReplaceAll(path, "/", `\`)

			log.Println(path)

			subfolderFiles, err := smbGetShareFiles(smbShare, path)
			if err != nil {
				log.Printf("could not read folder : %v", err)
			} else {
				sf = append(sf, subfolderFiles...)
			}
		}

		sf = append(sf, ShareFile{
			File:   file,
			Folder: folder,
		})
	}

	return sf, nil
}

func smbGetFiles(s *smb2.Session) ([]ShareFile, error) {
	var shareFiles []ShareFile

	names, err := s.ListSharenames()
	if err != nil {
		return shareFiles, fmt.Errorf("unable to list shares: %v", err)
	}

	for _, name := range names {
		log.Printf("enumerating share %v", name)

		fs, err := s.Mount(name)
		if err != nil {
			log.Printf("could not mount %v: %v", name, err)
			continue
		}

		files, err := smbGetShareFiles(fs, ".")
		fs.Umount()

		if err != nil {
			log.Printf("could not get share files from %v: %v", name, err)
			continue
		}

		for _, file := range files {
			file.ShareName = name
			shareFiles = append(shareFiles, file)
		}
	}

	return shareFiles, nil
}

func main() {
	server := flag.String("server", "", "smb server")
	user := flag.String("user", "", "NTLM user")
	pass := flag.String("pass", "", "NTLM pass")
	dbname := flag.String("dbname", "sqlite.db", "sqlite filename")
	flag.IntVar(&MaxDepth, "maxdepth", 3, "max recursion depth when retrieving files")

	flag.Parse()

	if *server == "" || *user == "" || *pass == "" {
		fmt.Fprintf(os.Stderr, "please specify a server, user and password")
		os.Exit(1)
	}

	log.Printf("max depth: %v", MaxDepth)

	db, err := connectAndSetup(*dbname)
	if err != nil {
		log.Fatalf("unable to create db: %v", err)
	}

	conn, s, err := smbSession(*server, *user, *pass)
	if err != nil {
		log.Fatalf("unable to connect to %v: %v", *server, err)
	}

	defer conn.Close()
	defer s.Logoff()

	files, err := smbGetFiles(s)
	if err != nil {
		log.Fatalf("unable to retrieve files: %v", err)
	}

	log.Printf("retrieved %v files", len(files))

	for _, file := range files {
		if err = addFile(db, *server, file); err != nil {
			log.Printf("unable to add file: %v", err)
		}
	}
}
