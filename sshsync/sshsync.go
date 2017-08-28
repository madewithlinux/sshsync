package main

import (
	"github.com/Joshua-Wright/sshsync"
	"os"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "-server" {
		//fmt.Println("server")
		sshsync.ServerMain()
	} else {
		//fmt.Println("client")
		sshsync.ClientMain()
	}
}
