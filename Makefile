all: 
	go build -ldflags "-X main.commitHash=$$(git rev-parse --short HEAD) -X main.commitDate=$$(git log -1 --format=%ct)"
