package main

import (
	"bufio"
	"database/sql"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hirochachacha/go-smb2"
	log "github.com/sirupsen/logrus"
)

var MaxDepth int
var timeout int

var db *sql.DB

type ShareFile struct {
	ServerName string
	ShareName  string
	File       fs.FileInfo
	Folder     string
}

var (
	// filled during compile time
	commitHash string
	commitDate string

	filesCount   uint64
	foldersCount uint64
	sharesCount  uint64
	serversCount uint64
)

func main() {
	var worker int
	var debugMode bool
	var excludeShares []string

	server := flag.String("server", "", "smb server")
	user := flag.String("user", "", "NTLM user")
	pass := flag.String("pass", "", "NTLM pass")
	dbname := flag.String("dbname", "sqlite.db", "sqlite filename")

	// ldap specific options
	ldapServer := flag.String("ldapServer", "", "ldap server to get smb server list")
	ldapDn := flag.String("ldapDn", "", "ldap distinguished name")
	ldapFilter := flag.String("ldapFilter", "(OperatingSystem=*server*)", "ldap filter to search for shares")

	excludeSharesList := flag.String("excludeShares", "", "share names to exclude, separated by a ','")

	flag.IntVar(&MaxDepth, "maxdepth", 3, "max recursion depth when retrieving files")
	flag.IntVar(&worker, "worker", 8, "amount of parallel worker")
	flag.IntVar(&timeout, "timeout", 5, "smb server connect timeout")
	flag.BoolVar(&debugMode, "debug", false, "debug mode")

	flag.Parse()

	if *pass == "" {
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("Enter password: ")
		pw, _ := reader.ReadString('\n')
		*pass = strings.TrimSpace(pw)
	}

	log.Infof("max depth: %v", MaxDepth)
	log.Infof("worker: %v", worker)
	log.Infof("timeout: %v", timeout)
	log.Infof("excluding shares: %v", excludeShares)

	if debugMode {
		log.SetLevel(log.DebugLevel)
	}

	log.SetFormatter(&log.TextFormatter{
		DisableColors: true,
		FullTimestamp: false,
	})

	// setup TUI app and connect logger
	tuiApp, logWriter := renderTui()

	log.SetOutput(logWriter)

	if *excludeSharesList != "" {
		excludeShares = strings.Split(*excludeSharesList, ",")
	}

	// run main procedure in goroutine because of TUI
	go start(MaxDepth, worker, timeout, *dbname, *server, *user, *pass, *ldapServer, *ldapDn, *ldapFilter, excludeShares)

	if err := tuiApp.Run(); err != nil {
		panic(err)
	}
}

func start(maxDepth, worker, timeout int, dbname, server, user, pass, ldapServer, ldapDn, ldapFilter string, excludeShares []string) {
	var err error

	defer func() {
		log.Info("quit")
	}()

	db, err = connectAndSetup(dbname)
	if err != nil {
		log.WithField("error", err).Fatal("unable to create db")
	}

	var serverlist []string

	if server != "" {
		serverlist = append(serverlist, server)
	}

	if ldapServer != "" {
		if ldapDn == "" {
			log.Error("please specify a -ldapDn")
			return
		}

		splitted := strings.SplitN(ldapDn, "DC", 2)
		if len(splitted) <= 1 {
			log.Error("invalid DN, could not extract baseDn")
			return
		}

		baseDn := fmt.Sprintf("DC%v", splitted[1])

		log.WithFields(log.Fields{
			"ldapServer": ldapServer,
			"ldapDn":     ldapDn,
			"ldapBaseDn": baseDn,
			"ldapFilter": ldapFilter,
		}).Info("querying LDAP")

		servers, err := getLdapServers(ldapServer, ldapDn, pass, baseDn, ldapFilter)
		if err != nil {
			log.Errorf("failed getting servers via ldaps: %v", err)
			return
		}

		log.WithField("serverCount", len(servers)).Info("retrieved serverlist from LDAP")

		serverlist = append(serverlist, servers...)
	}

	// semaphore for concurrency
	semaphore := make(chan int, worker)

	// writer goroutine for synchronous sqlite writes
	writer := make(chan ShareFile)

	// go routine for writing results to db
	go func() {
		for sf := range writer {
			if err = addFile(db, sf); err != nil {
				log.WithField("error", err).Error("unable to save to sqlite")
			}
		}
	}()

	// waitgroup to wait for all goroutines to finish
	var wg sync.WaitGroup

	for _, s := range serverlist {
		semaphore <- 1
		wg.Add(1)

		go func(s string, user string, pass string) {
			defer wg.Done()
			defer func() {
				atomic.AddUint64(&serversCount, 1)
				<-semaphore
			}()

			log.WithField("server", s).Infof("starting enumeration")

			if err := enumerateServer(s, user, pass, excludeShares, writer); err != nil {
				log.WithFields(log.Fields{
					"error":  err,
					"server": s,
				}).Warn("stopped enumeration")
				return
			}

			log.WithField("server", s).Info("finished enumeration")

		}(s, user, pass)

	}

	wg.Wait()
	close(writer)

	log.Infof("finished enumeration")
}

func smbSession(server, user, password string) (net.Conn, *smb2.Session, error) {
	dialer := net.Dialer{Timeout: time.Duration(timeout) * time.Second}
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

func smbGetShareFiles(smbShare *smb2.Share, folder, shareName, serverName string, writer chan ShareFile) error {
	if strings.Count(folder, `\`) > MaxDepth {
		return fmt.Errorf("max depth %v reached", MaxDepth)
	}

	f, err := smbShare.ReadDir(folder)
	if err != nil {
		return fmt.Errorf("could not open folder %v: %v", folder, err)
	}

	for _, file := range f {

		if file.IsDir() {
			atomic.AddUint64(&foldersCount, 1)

			path := filepath.Join(folder, file.Name())
			path = strings.ReplaceAll(path, "/", `\`)

			log.Debugf("folder: %v", path)

			if err := smbGetShareFiles(smbShare, path, shareName, serverName, writer); err != nil {
				log.WithFields(log.Fields{
					"error":  err,
					"folder": path,
				}).Debug("could not read folder")
			}
		} else {
			atomic.AddUint64(&filesCount, 1)
		}

		writer <- ShareFile{
			ServerName: serverName,
			ShareName:  shareName,
			File:       file,
			Folder:     folder,
		}
	}

	return nil
}

func smbGetFiles(s *smb2.Session, serverName string, excludeShares []string, writer chan ShareFile) error {
	names, err := s.ListSharenames()
	if err != nil {
		return fmt.Errorf("unable to list shares: %v", err)
	}

	for _, name := range names {

		if contains(excludeShares, name) {
			log.WithFields(log.Fields{
				"sharename": name,
				"server":    serverName,
			}).Debugf("skipping excluded share")
			continue
		}

		isScanned, err := shareScanned(db, serverName, name)
		if err != nil {
			return fmt.Errorf("error getting scan count: %v", err)
		}

		if isScanned {
			log.WithFields(log.Fields{
				"share":  name,
				"server": serverName,
			}).Info("skipping share, already indexed")
			continue
		}

		atomic.AddUint64(&sharesCount, 1)
		log.WithField("share", name).Debug("indexing share")

		if err := addShare(db, serverName, name); err != nil {
			log.Errorf("error saving share: %v", err)
		}

		fs, err := s.Mount(name)
		if err != nil {
			log.Debugf("could not mount %v: %v", name, err)
			updateShare(db, serverName, name, "failed")
			continue
		}

		err = smbGetShareFiles(fs, ".", name, serverName, writer)
		fs.Umount()

		if err != nil {
			log.Debugf("could not get share files from %v: %v", name, err)
			updateShare(db, serverName, name, "failed")
			continue
		}

		updateShare(db, serverName, name, "finished")
	}

	return nil
}

func enumerateServer(server, user, pass string, excludeShares []string, writer chan ShareFile) error {
	var err error

	conn, smbSession, err := smbSession(server, user, pass)
	if err != nil {
		return fmt.Errorf("unable to connect to %v: %v", server, err)
	}

	defer conn.Close()
	defer smbSession.Logoff()

	return smbGetFiles(smbSession, server, excludeShares, writer)
}
func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}
