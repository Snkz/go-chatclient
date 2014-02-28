package main

/*
#include <inttypes.h>
#include "client_main.c"
extern int sendCtrlMsg(uint16_t msgType, uint16_t dataLen, char *data, char *resp);
extern int registerClient();
extern int initReciever();
extern char * getNameBuffer();
extern char * readChatMsg(char *name);
extern void shutdownClient();
extern int initConnections(char *hostName, u_int16_t tcp, u_int16_t udp, char *name);
*/
import "C"
import "flag"
import "fmt"
import "bufio"
import "os"
import "time"
import "strings"
import "net/http"
import "io/ioutil"

// optarg flags
var host = flag.String("h", "localhost", "")
var udpPort = flag.Int("u", 0, "")
var tcpPort = flag.Int("t", 0, "")
var nick = flag.String("n", "", "")
var LOCSERV = flag.Bool("D", false, "")
var TERMINAL = flag.Bool("noX", false, "")

// Current Room
var currRoom = C.CString("")

const CLIENT_MSG = 0 // Regular Client mesg
const SWITCH_MSG = 1 // Switch to room mesg
const MEMBER_MSG = 2 // mesg contains members in room
const ROOMSL_MSG = 3 // mesg contains available rooms
const USQUIT_MSG = 4 // user has quit, no mesg
const CREATE_MSG = 5 // new room added, mesg on failure
const RESTAR_MSG = 6 // restart the server connection
const KEEPAL_MSG = 7 // send a generic message type
const ERRORI_MSG = 8 // bad input 

// format for gui to display
type Message struct {
	name    string
	mesg    string
	control int
}

/*
 * Read from standardin and write output to either
 * c chan or d chan based on the prefix "!"
 */
func readStdIn(c chan string, d chan string) {
	stdIn := bufio.NewReader(os.Stdin)
	data, _ := stdIn.ReadString('\n')
	for data != "" {
		if strings.HasPrefix(data, "!") {
			d <- strings.TrimRight(data, "\n") + " "
		} else if strings.HasPrefix(data, "Server:") {
			// drop it
		} else {
			c <- strings.TrimRight(data, "\n")
		}
		data, _ = stdIn.ReadString('\n')
	}
}

/*
 * read from c and chat with server
 */
func readChat(c chan string) {
	data := <-c
	for data != "" {
		chat(data)
		data = <-c
	}
}

/*
 * read from server and write to c 
 */
func readServer(c chan Message) {
	for {

		cname := C.getNameBuffer()
		data := C.GoString(C.readChatMsg(cname))
		name := C.GoString(cname)
		c <- Message{name, "\033[1m" + data + "\033[0m", CLIENT_MSG}

	}
}

/*
 * read from c, send to server and write to d
 */
func readControl(c chan string, d chan Message) {
	data := <-c
	ctrl_type := 0
	for data != "" {
		output := control(data, &ctrl_type)
		if output != "" {
			d <- Message{"Server", "\033[1m" + output + "\033[0m", ctrl_type}
		}
		data = <-c
	}
}

/*
 * read from c write to stdout
 */
func writeStdOut(c chan Message) {
	data := <-c
	for {
		fmt.Printf("%s: %s\n", data.name, data.mesg)
		data = <-c
	}
}

/*
 * Location Server
 */
func getLocationServer() {
	resp, err := http.Get("http://www.cdf.toronto.edu/~csc469h/fall/chatserver.txt")
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Sscanf(string(data), "%s %d %d", host, tcpPort, udpPort)
}

/*
 * Initilize the underlying C structures
 * All ports are opened here and the reciever is prepared
 * Server registration is also done here
 */
func initClient() int {

	port := C.initReciever()

	if port == -1 {
		fmt.Println("error during initialization of reciever")
		return -1
	}

	err := C.initConnections(C.CString(*host),
		C.uint16_t(*tcpPort),
		C.uint16_t(*udpPort),
		C.CString(*nick))

	if err == -1 {
		// print message to GUI and exit
		fmt.Println("error during initialization.")
		return -1
	}

	err = C.registerClient()

	if err == -1 {
		fmt.Println("error during registration.")
		return -1
	}

	fmt.Println("Success!")
	return 0
}

/*
 * send a string to the server
 */
func chat(data string) {
	dataLen := C.uint16_t(len(data))
	cdata := C.CString(data)
	err := C.sendChatMsg(dataLen, cdata)
	if err == -1 {
		fmt.Println("error during chat send.")
	}
}

/*
 * send a control string to the server, return the reply
 */
func control(data string, ctrl_type *int) string {

	ctrl := strings.Split(data, " ")[0]

	dataLen := C.uint16_t(len(""))
	cdata := C.CString("")
	output := C.CString("")

	data = strings.TrimLeft(data, ctrl)
	data = strings.Trim(data, " ")

	// Switch between all ctrl types, two of them are added for fault tolerance
	switch ctrl {
	case "!r":
		*ctrl_type = ROOMSL_MSG
		output = C.requestInfo(C.ROOM_LIST_REQUEST, cdata, dataLen, C.ROOM_LIST_SUCC)
	case "!c":
		*ctrl_type = CREATE_MSG
		dataLen = C.uint16_t(len(data))
		cdata = C.CString(data)
		output = C.requestInfo(C.CREATE_ROOM_REQUEST, cdata, dataLen, C.CREATE_ROOM_SUCC)
		if C.GoString(output) == "" {
			output = cdata
		}
	case "!s":
		*ctrl_type = SWITCH_MSG
		dataLen = C.uint16_t(len(data))
		cdata = C.CString(data)
		output = C.requestInfo(C.SWITCH_ROOM_REQUEST, cdata, dataLen, C.SWITCH_ROOM_SUCC)
		if C.GoString(output) == "" {
			output = cdata
			currRoom = cdata
		}
	case "!m":
		*ctrl_type = MEMBER_MSG
		dataLen = C.uint16_t(len(data))
		cdata = C.CString(data)
		output = C.requestInfo(C.MEMBER_LIST_REQUEST, cdata, dataLen, C.MEMBER_LIST_SUCC)
	case "!q":
		*ctrl_type = USQUIT_MSG
		C.requestInfo(C.QUIT_REQUEST, cdata, dataLen, C.QUIT_REQUEST)
		fmt.Println("You have quit, goodbye")
		C.shutdownClient()
		os.Exit(0)
	case "!k":
		*ctrl_type = KEEPAL_MSG
		output = C.requestInfo(C.MEMBER_KEEP_ALIVE, cdata, dataLen, C.MEMBER_KEEP_ALIVE)
	case "!x":
		*ctrl_type = RESTAR_MSG
		fmt.Println("Did you just ask for a restart? CAUSE THATS WHAT YOURE GETTING")
		C.shutdownClient()
		redoInit()
	default:
		*ctrl_type = ERRORI_MSG
		output = C.CString("operation could not be completed")
	}

	// Read the output from our ctrl if null, then we dont need 
	// to display anything, if boo then the server died
	// Return the message from the server
	if output == nil {
		// No output due to either bad ctrl or no output 
		return ""
	} else if C.GoString(output) == "boo" {
		// jesus the server is dead
		C.shutdownClient()
		redoInit()
		return ""
	}

	return C.GoString(output)
}

/*
 * Try to recconect twice, once immediatly, the other in a second with a different name
 * Switch to room that you were last in
 */
func redoInit() {
	if *LOCSERV {
		getLocationServer()
	}
	err := initClient()
	time.Sleep(time.Second)
	if err == -1 {
		*nick = "_" + *nick
		fmt.Printf("retrying with name: %s\n", *nick)
		C.shutdownClient()
		err = initClient()
	} else {
		// auto connect to the room
		C.requestInfo(C.SWITCH_ROOM_REQUEST, currRoom, C.uint16_t(10), C.SWITCH_ROOM_SUCC)
	}

	if err == -1 {
		fmt.Println("Failed to restart connection, Goodbye")
		C.shutdownClient()
		os.Exit(0)

	} else {
		// auto connect to the room
		C.requestInfo(C.SWITCH_ROOM_REQUEST, currRoom, C.uint16_t(10), C.SWITCH_ROOM_SUCC)

	}

}

/*
 * Checks to see if the server is still around periodically 
 */
func heartBeatChecker(c chan string) {
	for {
		time.Sleep(4 * 1e9)
		c <- "!k"
	}
}

/*
 * Main run method, spawn all go threads after the initial init command is done
 * Sleep indefinetly after 
 */
func main() {

	// TODO: enforce lenth of nick. 
	flag.Parse()
	if *LOCSERV {
		getLocationServer()
		fmt.Printf("Using Location Server host: %s, tcp: %d, udp: %d\n", *host, *tcpPort, *udpPort)
	}

	err := initClient()
	if err == -1 {
		return
	}

	serverData := make(chan Message)
	clientData := make(chan string)
	ctrlData := make(chan string)

	if *TERMINAL {
		go readStdIn(clientData, ctrlData)
		go writeStdOut(serverData)
	} else {
		go LaunchGUI(serverData, clientData, ctrlData)
	}

	go readChat(clientData)
	go readControl(ctrlData, serverData)
	go readServer(serverData)
	go heartBeatChecker(ctrlData)
	go heartBeatChecker(ctrlData)

	for {
		time.Sleep(4 * 1e9)
	}

}
