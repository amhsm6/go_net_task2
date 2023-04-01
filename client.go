package main

import (
    "github.com/gotk3/gotk3/gtk"
    "github.com/gotk3/gotk3/gdk"
    "github.com/gotk3/gotk3/glib"
    "github.com/gotk3/gotk3/cairo"
    "net"
    "os"
    "fmt"
    "log"
    "encoding/json"
    "encoding/binary"
    "runtime"
    "time"
    "math"
)

type UserMeta struct {
    Name string
    Id int
}

var serverResponse chan []byte
var conn *net.Conn

var lockGamesListUpdater chan struct{}
var unlockGamesListUpdater chan struct{}

var waitFor2Player chan struct{}
var invitationReceived chan []byte

var pistolChan chan struct{}
var loseChan chan struct{}
var winChan chan struct{}

func gamesListUpdater(box *gtk.Box) {
    timer := time.After(time.Millisecond)

    for {
        select {

        case <-timer:
            timer = time.After(time.Millisecond * 500)

            glib.IdleAdd(func() {
                box.GetChildren().Foreach(func (item any) {
                    widget, _ := item.(*gtk.Widget)
                    widget.Destroy()
                })

                playButton, err := gtk.ButtonNewWithLabel("Play")

                if err != nil {
                    log.Panic(err)
                }

                box.Add(playButton)

                gamesListBox, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 12)

                if err != nil {
                    log.Panic(err)
                }

                (*conn).Write([]byte{ byte(0) })

                buf := <-serverResponse

                var gamesList []UserMeta
                err = json.Unmarshal(buf, &gamesList)

                if err != nil {
                    log.Panic(err)
                }

                for _, userMeta := range gamesList {
                    button, err := gtk.ButtonNewWithLabel(userMeta.Name)

                    if err != nil {
                        log.Panic(err)
                    }

                    id := userMeta.Id
                    button.Connect("clicked", func() {
                        lockGamesListUpdater <- struct{}{}

                        buf = make([]byte, 5)
                        buf[0] = 1
                        binary.LittleEndian.PutUint32(buf[1:], uint32(id))

                        (*conn).Write(buf)

                        waitFor2Player <- struct{}{}
                    })

                    gamesListBox.Add(button)
                }

                box.Add(gamesListBox)

                box.ShowAll()
            })

        case <-lockGamesListUpdater:
            <-unlockGamesListUpdater

        }
    }
}

func startGame(box *gtk.Box, buf []byte) {
    fmt.Println("STARTING GAME")

    enemy_x := binary.LittleEndian.Uint32(buf[:4])
    enemy_y := binary.LittleEndian.Uint32(buf[4:])

    fmt.Printf("Enemy coordinates: (%v, %v)\n", enemy_x, enemy_y)

    pistolChan = make(chan struct{}, 1)

    go func() {
        <-pistolChan
        pistolChan <- struct{}{}

        fmt.Println("Enemy picked pistol")

        glib.IdleAdd(func() {
            box.QueueDraw()
        })
    }()

    glib.IdleAdd(func () {
        box.GetChildren().Foreach(func (item any) {
            widget, _ := item.(*gtk.Widget)
            widget.Destroy()
        })

        drawing_area, err := gtk.DrawingAreaNew()

        if err != nil {
            log.Panic(err)
        }

        drawing_area.SetSizeRequest(800, 600)
        drawing_area.AddEvents(int(gdk.BUTTON_PRESS_MASK))

        drawing_area.Connect("draw", func(da *gtk.DrawingArea, ctx *cairo.Context) {
            select {

            case <-pistolChan:
                pistolChan <- struct{}{}
                ctx.SetSourceRGB(0, 255, 0)

            default:
                ctx.SetSourceRGB(255, 0, 0)

            }

            ctx.Arc(float64(enemy_x), float64(enemy_y), 20, 0, 2 * math.Pi)
            ctx.Fill()
        })

        drawing_area.Connect("button-press-event", func(da *gtk.DrawingArea, event *gdk.Event) {
            button_event := gdk.EventButtonNewFromEvent(event)

            buf := make([]byte, 9)

            buf[0] = 5
            binary.LittleEndian.PutUint32(buf[1:], uint32(button_event.X()))
            binary.LittleEndian.PutUint32(buf[5:], uint32(button_event.Y()))

            (*conn).Write(buf)
        })

        box.Add(drawing_area)

        button, err := gtk.ButtonNewWithLabel("Take pistol")

        if err != nil {
            log.Panic(err)
        }

        button.Connect("clicked", func() {
            (*conn).Write([]byte{ 4 })
        })

        box.Add(button)

        box.ShowAll()
        box.QueueDraw()
    })
}

func winHandler(box *gtk.Box) {
    for {
        <-winChan

        fmt.Println("You won")

        glib.IdleAdd(func () {
            box.GetChildren().Foreach(func (item any) {
                widget, _ := item.(*gtk.Widget)
                widget.Destroy()
            })

            label, err := gtk.LabelNew("You won")

            if err != nil {
                log.Panic(err)
            }

            box.Add(label)

            box.ShowAll()
        })

        time.Sleep(4 * time.Second)
        unlockGamesListUpdater <- struct{}{}
    }
}

func loseHandler(box *gtk.Box) {
    for {
        <-loseChan

        fmt.Println("You lose")

        glib.IdleAdd(func () {
            box.GetChildren().Foreach(func (item any) {
                widget, _ := item.(*gtk.Widget)
                widget.Destroy()
            })

            label, err := gtk.LabelNew("You lose")

            if err != nil {
                log.Panic(err)
            }

            box.Add(label)

            box.ShowAll()
        })

        time.Sleep(4 * time.Second)
        unlockGamesListUpdater <- struct{}{}
    }
}

func awaitInvitation(box *gtk.Box) {
    for {
        buf := <-invitationReceived
        lockGamesListUpdater <- struct{}{}

        glib.IdleAdd(func() {
            box.GetChildren().Foreach(func (item any) {
                widget, _ := item.(*gtk.Widget)
                widget.Destroy()
            })

            label, err := gtk.LabelNew(fmt.Sprintf("User %v invites you!", string(buf)))

            if err != nil {
                log.Panic(err)
            }

            box.Add(label)

            acceptButton, err := gtk.ButtonNewWithLabel("Accept")

            if err != nil {
                log.Panic(err)
            }

            acceptButton.Connect("clicked", func() {
                (*conn).Write([]byte{ 3 })

                buf = <-serverResponse

                if buf[0] == 1 {
                    startGame(box, buf[1:])
                }
            })

            box.Add(acceptButton)

            refuseButton, err := gtk.ButtonNewWithLabel("Refuse")

            if err != nil {
                log.Panic(err)
            }

            refuseButton.Connect("clicked", func() {
                unlockGamesListUpdater <- struct{}{}

                (*conn).Write([]byte{ 6 })
            })

            box.Add(refuseButton)

            box.ShowAll()
        })
    }
}

func awaitInvitationAccept(box *gtk.Box) {
    for {
        <-waitFor2Player

        buf := <-serverResponse

        if buf[0] == 1 {
            startGame(box, buf[1:])
        } else if buf[0] == 5 {
            unlockGamesListUpdater <- struct{}{}
        } else {
            log.Panic("UNKNOWN COMMAND", buf)
        }
    }
}

func serverRecv() {
    for {
        buf := make([]byte, 1000)
        n, _ := (*conn).Read(buf)

        if buf[0] == 0 {
            invitationReceived <- buf[1:n]
        } else if buf[0] == 2 {
            pistolChan <- struct{}{}
        } else if buf[0] == 3 {
            loseChan <- struct{}{}
        } else if buf[0] == 4 {
            winChan <- struct{}{}
        } else {
            serverResponse <- buf[:n]
        }
    }
}

func main() {
    if len(os.Args) < 2 {
        fmt.Println("Server address needed")
        os.Exit(1)
    }

    runtime.LockOSThread()
    gtk.Init(nil)

    win, err := gtk.WindowNew(gtk.WINDOW_TOPLEVEL)

    if err != nil {
        log.Panic(err)
    }

    win.Connect("destroy", func() {
        gtk.MainQuit()
    })

    box, err := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 12)

    if err != nil {
        log.Panic(err)
    }

    nameLabel, err := gtk.LabelNew("Enter name");

    if err != nil {
        log.Panic(err)
    }

    box.Add(nameLabel)

    nameInput, err := gtk.EntryNew()

    if err != nil {
        log.Panic(err)
    }

    nameInput.Connect("activate", func(input *gtk.Entry) {
        text, err := input.GetText()

        if err != nil {
            log.Panic(err)
        }

        fmt.Println("Your name is", text)

        conn2, err := net.Dial("tcp", os.Args[1])

        if err != nil {
            log.Panic(err)
        }

        conn = &conn2

        (*conn).Write([]byte(text))

        lockGamesListUpdater = make(chan struct{})
        unlockGamesListUpdater = make(chan struct{})
        go gamesListUpdater(box)

        waitFor2Player = make(chan struct{})
        go awaitInvitationAccept(box)

        invitationReceived = make(chan []byte)
        go awaitInvitation(box)

        serverResponse = make(chan []byte)
        go serverRecv()

        loseChan = make(chan struct{})
        go loseHandler(box)

        winChan = make(chan struct{})
        go winHandler(box)

        box.ShowAll()
    })

    box.Add(nameInput)

    win.Add(box)

    win.ShowAll()

    gtk.Main()
}
