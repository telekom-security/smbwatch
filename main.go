package main

import (
	"flag"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hirochachacha/go-smb2"
	log "github.com/sirupsen/logrus"
)

var MaxDepth int
var timeout = time.Second * 10

type ShareFile struct {
	ShareName string
	File      fs.FileInfo
	Folder    string
}

func smbSession(server, user, password string) (net.Conn, *smb2.Session, error) {
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.Dial("tcp", fmt.Sprintf("%v:445", server))
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
		return sf, fmt.Errorf("max depth %v reached", MaxDepth)
	}

	f, err := smbShare.ReadDir(folder)
	if err != nil {
		return sf, fmt.Errorf("could not open folder %v: %v", folder, err)
	}

	for _, file := range f {

		if file.IsDir() {
			path := filepath.Join(folder, file.Name())
			path = strings.ReplaceAll(path, "/", `\`)

			log.Debugf("folder: %v", path)

			subfolderFiles, err := smbGetShareFiles(smbShare, path)
			if err != nil {
				log.WithFields(log.Fields{
					"error":  err,
					"folder": path,
				}).Debug("could not read folder")
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
		log.WithField("share", name).Debug("indexing share")

		fs, err := s.Mount(name)
		if err != nil {
			log.Debugf("could not mount %v: %v", name, err)
			continue
		}

		files, err := smbGetShareFiles(fs, ".")
		fs.Umount()

		if err != nil {
			log.Debugf("could not get share files from %v: %v", name, err)
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
	var worker int
	var debugMode bool

	server := flag.String("server", "", "smb server")
	user := flag.String("user", "", "NTLM user")
	pass := flag.String("pass", "", "NTLM pass")
	dbname := flag.String("dbname", "sqlite.db", "sqlite filename")

	// ldap specific options
	ldapServer := flag.String("ldapServer", "", "ldap server to get smb server list")
	ldapDn := flag.String("ldapDn", "", "ldap distinguished name")
	ldapFilter := flag.String("ldapFilter", "(OperatingSystem=*server*)", "ldap filter to search for shares")

	flag.IntVar(&MaxDepth, "maxdepth", 3, "max recursion depth when retrieving files")
	flag.IntVar(&worker, "worker", 8, "amount of parallel worker")
	flag.BoolVar(&debugMode, "debug", false, "debug mode")

	flag.Parse()

	if debugMode {
		log.SetLevel(log.DebugLevel)
	}

	if *user == "" || *pass == "" {
		fmt.Fprintf(os.Stderr, "please specify a user and password")
		os.Exit(1)
	}

	log.Infof("max depth: %v", MaxDepth)
	log.Infof("worker: %v", MaxDepth)

	db, err := connectAndSetup(*dbname)
	if err != nil {
		log.WithField("error", err).Fatal("unable to create db")
	}

	var serverlist []string

	if *server != "" {
		serverlist = append(serverlist, *server)
	}

	if *ldapServer != "" {
		if *ldapDn == "" {
			log.Fatalf("please specify a -ldapDn")
		}

		splitted := strings.SplitN(*ldapDn, "DC", 2)
		if len(splitted) <= 1 {
			log.Fatalf("invalid DN, could not extract baseDn")
		}

		baseDn := fmt.Sprintf("DC%v", splitted[1])

		log.WithFields(log.Fields{
			"ldapServer": *ldapServer,
			"ldapDn":     *ldapDn,
			"ldapBaseDn": baseDn,
			"ldapFilter": *ldapFilter,
		}).Info("querying LDAP")

		servers, err := getLdapServers(*ldapServer, *ldapDn, *pass, baseDn, *ldapFilter)
		if err != nil {
			log.Fatalf("failed getting servers via ldaps: %v", err)
		}

		log.WithField("serverCount", len(servers)).Info("retrieved serverlist from LDAP")

		serverlist = append(serverlist, servers...)
	}

	// using a semaphore here will limit max. concurrency
	semaphore := make(chan int, worker)

	for _, s := range serverlist {
		semaphore <- 1

		go func(s string, user string, pass string) {
			log.WithField("server", s).Infof("starting enumeration")

			files, err := enumerateServer(s, user, pass)
			if err != nil {
				log.WithFields(log.Fields{
					"error":  err,
					"server": s,
				}).Warn("unable to enumerate")
				return
			}

			log.Printf("retrieved %v files", len(files))

			for _, file := range files {
				if err = addFile(db, s, file); err != nil {
					log.WithField("error", err).Error("unable to save file to sqlite", err)
				}
			}

			<-semaphore
		}(s, *user, *pass)

	}
}

func enumerateServer(server, user, pass string) ([]ShareFile, error) {
	var sharefiles []ShareFile
	var err error

	conn, s, err := smbSession(server, user, pass)
	if err != nil {
		return sharefiles, fmt.Errorf("unable to connect to %v: %v", server, err)
	}

	defer conn.Close()
	defer s.Logoff()

	sharefiles, err = smbGetFiles(s)
	if err != nil {
		return sharefiles, err
	}

	return sharefiles, nil
}
