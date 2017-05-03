package main

import (
    "bufio"
    "fmt"
    "net"
    "os"
    "strings"
    "sync"
    "time"
)

var serverAddr string = "127.0.0.1"
var serverPort string = "4956"

var syncInterval int = 3 // seconds
var maxClients uint = 50 // seconds

var maxOffset time.Duration = 5 // seconds

var verboseMessages bool = false

type client_s struct {
    number   string
    addr     string
    port     string
    pullTime time.Time
    offset   time.Duration
    time     time.Time
    active   bool
}

var clients []client_s
var mutex *sync.Mutex

var defaultOffsetString string
var defaultOffset time.Duration
var timeOffset time.Duration = 0

func main() {

    // Get args
    defaultOffsetString = os.Args[1]
    offset, err := time.ParseDuration(defaultOffsetString)
    exitOnError(err)
    defaultOffset = offset

    clients = make([]client_s, 0, maxClients)

    // Init mutex
    mutex = &sync.Mutex{}

    message("Launching server on port", serverPort, "with", defaultOffsetString, "offset")

    // Listen for clients
    go listenForNewClients()

    // Sync every 10 seconds
    for {
        for i := 0; i < syncInterval; i++ {
            verboseMessage("Will sync in", syncInterval-i)
            time.Sleep(time.Second)
        }
        go syncClocks()
        time.Sleep(time.Second)
    }
}

func listenForNewClients() {
    verboseMessage("Listen for clients...")
    ln, err := net.Listen("tcp", serverAddr+":"+serverPort)
    exitOnError(err)
    for {
        verboseMessage("Wait for new clients...")
        conn, err := ln.Accept()
        exitOnError(err)
        verboseMessage("Got a new client")
        go handleNewClientConnection(conn)
    }

    // Should never reach this
    message("Done")
}

func handleNewClientConnection(conn net.Conn) {
    verboseMessage("Handle new client connection...")

    str, err := bufio.NewReader(conn).ReadString('\n')
    exitOnError(err)
    if len(str) > 1 {
        if addClient(str[:len(str)-1]) {
            conn.Write([]byte("Client added\n"))
        } else {
            conn.Write([]byte("Client already exists\n"))
        }
    } else {
        conn.Write([]byte("Invalid message\n"))
    }

    conn.Close()
}

func addClient(newClientString string) bool {

    split := strings.Split(newClientString, " ")

    newClient := client_s{
        number: split[0],
        addr:   split[1],
        port:   split[2],
        active: true,
    }

    mutex.Lock()
    for i := 0; i < len(clients); i++ {
        // Skip if the thing already exists
        if clients[i].addr == newClient.addr && clients[i].port == newClient.port {
            if !clients[i].active {
                clients[i].active = true
                message("Client", clients[i].number, "reactivated: "+newClient.addr+":"+newClient.port)
                mutex.Unlock()
                return true
            }
            mutex.Unlock()
            return false
        }
    }

    // Append
    clients = append(clients, newClient)
    mutex.Unlock()
    message("Client " + newClient.number + " added: " + newClient.addr + ":" + newClient.port)
    return true
}

func syncClocks() {

    message("")
    message("Sync:")
    mutex.Lock()

    if len(clients) == 0 {
        message("No clients to sync!")
        mutex.Unlock()
        return
    }

    // refTime := getCurrentTime()

    var offsetSum time.Duration = 0
    var offsetNum time.Duration = 1

    message("[S]  Time:", getCurrentTime())

    // Request time
    for i := 0; i < len(clients); i++ {

        if !clients[i].active {
            continue
        }

        // Prepend messages with:
        m_error := "[C" + clients[i].number + "] ERR:"
        m_info := "[C" + clients[i].number + "]"

        // Connect
        conn, err := net.Dial("tcp", clients[i].addr+":"+clients[i].port)
        if err != nil {
            clients[i].active = false
            verboseMessage(m_error, err)
            message(m_error, "Client deactivated")
            continue
        }

        // Mark current time
        before := getCurrentTime()

        // Send request
        _, err = fmt.Fprintf(conn, "getCurrentTime\n")
        if err != nil {
            message(m_error, err)
            continue
        }

        // Wait for response
        str, err := bufio.NewReader(conn).ReadString('\n')

        // Measure response duration
        after := getCurrentTime()

        // Treat errors
        if err != nil {
            message(m_error, err)
            continue
        }
        if len(str) < 1 {
            message(m_error, "Invalid string!")
            continue
        }

        // Parse response
        received, err := time.Parse(time.RFC3339Nano, str[:len(str)-1])
        if err != nil {
            message(m_error, err)
            continue
        }
        message(m_info, "Received:", received)

        // Server pull time
        clients[i].pullTime = after

        // Approximate remote time
        clients[i].time = received.Add(after.Sub(before) / 2)

        // Offset
        clients[i].offset = clients[i].time.Sub(clients[i].pullTime)

        verboseMessage(m_info, "Actual time:", time.Now())
        verboseMessage(m_info, "Offset time:", getCurrentTime())
        verboseMessage(m_info, "Receiv time:", received)

        // Add to avg parts
        if clients[i].offset < maxOffset*time.Second && clients[i].offset > -maxOffset*time.Second {
            message(m_info, "Offset added: ", clients[i].offset)
            offsetSum += clients[i].offset
            offsetNum++
        } else {
            message(m_info, "Offset ignored. It's higher then", maxOffset*time.Second, ":", clients[i].offset)
        }

        conn.Close()
    }

    offsetAvg := offsetSum / offsetNum

    verboseMessage("")
    verboseMessage("Offsets num:", offsetNum)
    verboseMessage("Offsets sum:", offsetSum)
    verboseMessage("Offsets avg:", offsetAvg)

    message("")

    // Request time
    for i := 0; i < len(clients); i++ {

        if !clients[i].active {
            continue
        }

        // Prepend messages with:
        m_error := "[C" + clients[i].number + "] ERR:"
        m_info := "[C" + clients[i].number + "]"

        // Connect
        conn, err := net.Dial("tcp", clients[i].addr+":"+clients[i].port)
        if err != nil {
            clients[i].active = false
            verboseMessage(m_error, err)
            message(m_error, "Client deactivated")
            continue
        }

        offset := offsetAvg - clients[i].offset

        // Send request
        message(m_info, "Set offset:", offset)
        _, err = fmt.Fprintf(conn, "setOffset "+offset.String()+"\n")
        if err != nil {
            message(m_error, err)
            continue
        }

        conn.Close()
    }

    // Set local offset
    message("[S]  Set offset:", offsetAvg)
    timeOffset = timeOffset + offsetAvg

    mutex.Unlock()
    verboseMessage("syncClocks(): End")
}

func getCurrentTime() time.Time {
    return time.Now().Add(defaultOffset).Add(timeOffset)
}

func message(a ...interface{}) (n int, err error) {
    return fmt.Print("[S] ", fmt.Sprintln(a...))
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
