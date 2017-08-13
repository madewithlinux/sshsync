package main

import (
	"golang.org/x/crypto/ssh"
	"log"
	"fmt"
	"net"
	"os"
	"golang.org/x/crypto/ssh/agent"
	"io"
)

func main() {


	sock, err := net.Dial("unix", os.Getenv("SSH_AUTH_SOCK"))
	if err != nil {
		log.Fatal(err)
	}
	agent := agent.NewClient(sock)
	signers, err := agent.Signers()
	if err != nil {
		log.Fatal(err)
	}
	auths := []ssh.AuthMethod{ssh.PublicKeys(signers...)}


	fmt.Println(auths[0])

	config := &ssh.ClientConfig{
		User: "j0sh",
		Auth: auths,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	// Dial your ssh server.
	conn, err := ssh.Dial("tcp", "localhost:22", config)
	if err != nil {
		log.Fatal("unable to connect: ", err)
	}
	defer conn.Close()

	l, err := conn.Listen("tcp", "0.0.0.0:8080")
	if err != nil {
		log.Fatal("unable to register tcp forward: ", err)
	}
	defer l.Close()


	session, err := conn.NewSession()
	stdout, err := session.StdoutPipe()
	session.Start("echo test")
	io.Copy(os.Stdout, stdout)


	//// Serve HTTP with your SSH server acting as a reverse proxy.
	//http.Serve(l, http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
	//	fmt.Fprintf(resp, "Hello world!\n")
	//}))

}