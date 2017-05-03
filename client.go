package main

import (
    "bufio"
    "fmt"
    "net"
    "os"
    "time"
)

var serverAddr string = "127.0.0.1"
var serverPort string = "4956"

var clientAddr string = "127.0.0.1"
var clientPort string = "485"

var announceInterval time.Duration = 10 // seconds

var verboseMessages bool = false

var clientNumber string

var defaultOffsetString string
var defaultOffset time.Duration
var timeOffset time.Duration = 0

func main() {

    // Get args
    clientNumber = os.Args[1]
    defaultOffsetString = os.Args[2]
    offset, err := time.ParseDuration(defaultOffsetString)
    exitOnError(err)
    defaultOffset = offset

    // Set client port
    clientPort = clientPort + clientNumber

    // Do stuff
    message("Launching client", clientNumber, "on port", clientPort, "with", defaultOffsetString, "offset")

    go listenForServerRequests()

    for {
        go announceToServer()
        time.Sleep(announceInterval * time.Second)
    }
}

func announceToServer() {
    verboseMessage("Announcing to server...")
    for {
        conn, err := net.Dial("tcp", serverAddr+":"+serverPort)
        if err != nil {
            // message(err)
        } else {
            _, err := fmt.Fprintf(conn, clientNumber+" "+clientAddr+" "+clientPort+"\n")
            if err != nil {
                message(err)
                continue
            }

            str, err := bufio.NewReader(conn).ReadString('\n')
            if err != nil {
                message(err)
                continue
            }
            if str == "Client added\n" {
                message("Announced to server")
            } else if str == "Client already exists\n" {
                verboseMessage("Already announced")
            }

            break
        }
        time.Sleep(time.Second)
    }
}

func listenForServerRequests() {
    message("Listen for server requests...")
    ln, err := net.Listen("tcp", clientAddr+":"+clientPort)
    exitOnError(err)
    for {
        verboseMessage("Wait for request...")
        conn, err := ln.Accept()
        exitOnError(err)
        go handleRequest(conn)
    }
    verboseMessage("Done")
}

func handleRequest(conn net.Conn) {
    verboseMessage("Handle request...")

    str, err := bufio.NewReader(conn).ReadString('\n')
    exitOnError(err)
    if str == "getCurrentTime\n" {
        go message("Request: getCurrentTime")
        _, err := conn.Write([]byte(getCurrentTime().Format(time.RFC3339Nano) + "\n"))
        exitOnError(err)
    } else if str[:9] == "setOffset" {
        message("Request: setOffset", str[10:len(str)-1])
        offset, err := time.ParseDuration(str[10 : len(str)-1])
        exitOnError(err)
        timeOffset = timeOffset + offset
    } else {
        message("Invalid request:", str)
    }

    conn.Close()
}

func getCurrentTime() time.Time {
    return time.Now().Add(defaultOffset).Add(timeOffset)
}

func message(a ...interface{}) (n int, err error) {
    return fmt.Print("[C"+clientNumber+"] ", fmt.Sprintln(a...))
}

func verboseMessage(a ...interface{}) {
    if verboseMessages {
        message(a...)
    }
}

func exitOnError(err error) {
    if err != nil {
        message("ERR:", err)
        os.Exit(1)
    }
}
