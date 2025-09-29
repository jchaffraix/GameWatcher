.PHONY: all clean build run

all: build

build:
	go build -o watcher fanatical.go greenmangaming.go humblebundle.go loaded.go steam.go flags.go filter.go main.go

# TODO: If flag is defined, none of the argument building applies.
flag=
ifdef debug
flag+=-debug
endif
ifdef file
flag+=-file $(file)
else ifdef games
flag+=-games "$(games)"
endif

run: build
	./watcher $(flag)

clean:
	go clean
	rm watcher
