package main

import "fmt"
import "time"
import "unsafe"
import "strings"
import "github.com/mattn/go-gtk/gtk"
import "github.com/mattn/go-gtk/gdk"
import "github.com/mattn/go-gtk/glib"

// Utility method: Returns true if message is a control message
func isCtrlMessage(message string) bool {
	return strings.HasPrefix(message, "!")
}

// Utility method: Returns true of message is terminated by return
func isEndOfMessage(message string) bool {
	return strings.HasSuffix(message, "\n")
}

func toUtf8(iso8859_1_buf []byte) string {
	buf := make([]rune, len(iso8859_1_buf))
	for i, b := range iso8859_1_buf {
		buf[i] = rune(b)
	}
	return string(buf)
}

// handles creation and update of GUI components while sending
// data back and forth between the c handlers
func LaunchGUI(servData chan Message, cData chan string, ctrlData chan string) {

	gtk.Init(nil)
	window := gtk.NewWindow(gtk.WINDOW_TOPLEVEL)
	window.ModifyBG(window.GetState(), gdk.NewColor("#99ccff"))
	window.SetPosition(gtk.WIN_POS_CENTER)
	window.SetTitle("Le Distributed Chat Client ~")
	window.Connect("destroy", func(ctx *glib.CallbackContext) {
		// add any extra clean up code
		ctrlData <- "!q"
		gtk.MainQuit()
	})

	vbox := gtk.NewVBox(false, 1)
	menubar := gtk.NewMenuBar()
	vbox.PackStart(menubar, false, false, 0)

	vpaned := gtk.NewVPaned()
	vbox.Add(vpaned)

	// shows chat messages and server responses.
	msgFrame := gtk.NewFrame("")
	msgFrameBox := gtk.NewVBox(false, 1)
	msgFrame.Add(msgFrameBox)

	msgFrameBox.SetBorderWidth(5)
	// the user types in messages here.
	ctrlFrame := gtk.NewFrame(*nick)
	ctrlFrameBox := gtk.NewVBox(false, 1)
	ctrlFrame.Add(ctrlFrameBox)

	vpaned.Pack1(msgFrame, false, false)
	vpaned.Pack2(ctrlFrame, false, false)

	label := gtk.NewLabel("Distributed Chat Client")
	label.ModifyFontEasy("bold calibri 16 white")
	label.SetPadding(20, 20)
	msgFrameBox.PackStart(label, false, true, 0)

	sCtrlWin := gtk.NewScrolledWindow(nil, nil)
	sCtrlWin.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	sCtrlWin.SetShadowType(gtk.SHADOW_IN)
	ctrlTextView := gtk.NewTextView()

	ctrlTextViewBuffer := ctrlTextView.GetBuffer()
	ctrlTextView.ModifyFontEasy("bold calibri 16")
	sCtrlWin.Add(ctrlTextView)

	ctrlFrameBox.SetBorderWidth(5)
	ctrlFrameBox.Add(sCtrlWin)

	event := make(chan interface{})

	ctrlTextView.Connect("key-release-event", func(ctx *glib.CallbackContext) {
		arg := ctx.Args(0)
		event <- *(**gdk.EventKey)(unsafe.Pointer(&arg))
	})

	// go routine to handle cleanup for control text view
	go func() {
		var start gtk.TextIter
		var end gtk.TextIter
		for {
			e := <-event
			switch ev := e.(type) {
			case *gdk.EventKey:
				// enter key
				if 65293 == ev.Keyval {
					fmt.Println("user hit Enter beginning to parse input.")
					// if the user hits the return key then the message is sent 
					// over the channel and the ctrlTextView is cleared.
					ctrlTextViewBuffer.GetStartIter(&start)
					ctrlTextViewBuffer.GetEndIter(&end)
					userInput := ctrlTextViewBuffer.GetText(&start, &end, true)

					ctrlTextViewBuffer.SetText("")

					fmt.Println("User entered ", userInput)
					userInput = strings.TrimRight(userInput, "\n")
					userInput = strings.TrimLeft(userInput, "\n")
					userInput = strings.TrimRight(userInput, " ")
					userInput = strings.TrimLeft(userInput, " ")

					if isCtrlMessage(userInput) {
						fmt.Println("... is a control message, trimming and passing over ctrl channel.")
						ctrlData <- userInput

					} else {
						// if the input is a chat message send it over the client data channel.
						fmt.Println("... is a chat message, trimming and passing over user channel.")
						cData <- userInput
					}
				}
				break
			}
		}
	}()

	// scrollable window for displaying chat for a given 
	// room or for control responses from the server.
	sMsgWin := gtk.NewScrolledWindow(nil, nil)
	sMsgWin.SetPolicy(gtk.POLICY_AUTOMATIC, gtk.POLICY_AUTOMATIC)
	sMsgWin.SetShadowType(gtk.SHADOW_IN)
	msgTextView := gtk.NewTextView()

	// The user cannot edit this window
	msgTextView.SetEditable(false)
	msgTextViewBuffer := msgTextView.GetBuffer()

	sMsgWin.Add(msgTextView)
	msgFrameBox.Add(sMsgWin)

	msgTextViewBuffer.Connect("changed", func() {
		fmt.Println("changed")
	})

	// status bar
	statusbar := gtk.NewStatusbar()
	context_id := statusbar.GetContextId("go-gtk")
	statusbar.Push(context_id, "status messages go here")
	ctrlFrameBox.PackStart(statusbar, false, false, 0)

	// start a go routine to listen for messages coming in from
	// the server channel, then display these messages on the message window
	go func() {
		for {
			// Message struct 
			s := <-servData
			t := time.Now().Format(time.RubyDate)
			fmt.Print("received data from servData ", s)
			t = t[11:19]
			switch s.control {
			case CLIENT_MSG:
				var start gtk.TextIter
				// Regular Client mesg
				// display in msgTextViewl
				msgTextViewBuffer.GetStartIter(&start)
				mesg := s.mesg
				mesg = mesg[4 : len(mesg)-4]

				mesg = "[" + s.name + "] " + t + " : " + mesg
				msgTextViewBuffer.Insert(&start, mesg+"\n")

				statusbar.Pop(context_id)
				statusbar.Push(context_id, "Received message from "+s.name)

			case SWITCH_MSG:

				mesg := s.mesg
				mesg = mesg[4 : len(mesg)-4]

				if mesg != "Already in this room!" {
					gdk.ThreadsEnter()
					label.SetLabel(mesg)
					gdk.ThreadsLeave()
					statusbar.Push(context_id, "Received message from server")
				}

			case MEMBER_MSG:
				var start gtk.TextIter
				mesg := s.mesg
				mesg = mesg[4 : len(mesg)-4]
				// mesg contains members in room
				// print the members in the room in msgTextView
				msgTextViewBuffer.GetStartIter(&start)
				msgTextViewBuffer.Insert(&start, "["+s.name+"]: "+mesg+"\n")
				statusbar.Pop(context_id)
				statusbar.Push(context_id, "Received message from server")

			case ROOMSL_MSG:
				var start gtk.TextIter
				mesg := s.mesg
				mesg = mesg[4 : len(mesg)-4]
				// mesg contains available rooms
				// print available room in msgTextView
				msgTextViewBuffer.GetStartIter(&start)
				msgTextViewBuffer.Insert(&start, "["+s.name+"]: "+mesg+"\n")
				statusbar.Pop(context_id)
				statusbar.Push(context_id, "Received message from server")

			case CREATE_MSG:
				var start gtk.TextIter
				mesg := s.mesg
				mesg = mesg[4 : len(mesg)-4]
				// create a new room
				// display that a new room is created in msgTextView
				msgTextViewBuffer.GetStartIter(&start)
				msgTextViewBuffer.Insert(&start, "["+s.name+"]: "+mesg+"\n")
				statusbar.Pop(context_id)
				statusbar.Push(context_id, "Received message from server")

			case USQUIT_MSG:
				// user has quit, no mesg
				// kill GUI
				gtk.MainQuit()
			case KEEPAL_MSG:
				// invalid name
				statusbar.Pop(context_id)
				statusbar.Push(context_id, "ERROR: name invalid. Not connected to server.")
			case ERRORI_MSG:
				// write to status field: couldn't write
				statusbar.Pop(context_id)
				statusbar.Push(context_id, "Invalid operation performed!")
			}
		}
	}()

	window.Add(vbox)
	window.SetSizeRequest(600, 600)
	window.ShowAll()

	go gtk.Main()
}
