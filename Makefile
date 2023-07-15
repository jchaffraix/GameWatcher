all: build

build:
	go build -o watcher fanatical.go greenmangaming.go humblebundle.go steam.go flags.go main.go

clean:
	go clean
	rm watcher
