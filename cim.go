// cim is a command line interface manager package
// Using this package one can create a cim server as well as a client
package cim


import (
    "fmt"
    "net"
    "github.com/LDCS/qcfg"
    "errors"
    "bytes"
    "strings"
)

type CimConnection struct {
    host string
    simInst string
    port string
    tcpAddr *net.TCPAddr
    tcpConn *net.TCPConn
    resp []byte
}

// Callback accepts the shared data command, and arguments, returns a string
type CBfunc func (interface{}, string, ...string) (string)

//func test_func(data interface{}, cmd string, args ...string) (string) {
//    return cmd
//}

type ExecData struct {
    Callback CBfunc
    Cmd string
    Args []string
    Data interface{}
    RetChan chan string
}

type CimNode struct {
    IsLeaf bool
    Name string
    Path string
    Children []*CimNode
    Callbacks map[string]CBfunc
    Parent *CimNode
}

type CimServer struct {
    host string
    simInst string
    port string
    root *CimNode
    data interface{}
}

// The following cfg file contains the mapping from server names to ports
var cfgFile = "/home/dennis/devel/goprojects/src/github.com/LDCS/cim/ports.cfg"

const responseMaxLength = 1024*1024

func NewCimServer(host, simInst string, root *CimNode, data interface{}) (*CimServer, error) {
    cs := new(CimServer)
    if host == "" {
		host = "127.0.0.1"
    }
    cs.host = host
    cs.simInst = simInst
    cs.root = root
    cs.data = data
    var ccfg *qcfg.CfgBlock = qcfg.NewCfg(simInst, cfgFile, false)
    var port = ccfg.Str("ports-lst", "ports", simInst, "")
    if port == "" {
		return nil, errors.New("Invalid sim : " + simInst)
    }
    cs.port = port
    return cs, nil
}

func (cs *CimServer) Start() error {
    ln, err := net.Listen("tcp", cs.host + ":" + cs.port)
    if err != nil {
		return err
    }
    execChan := make(chan ExecData)
    // This go routine actually runs all non built-in commands from cim client
    go callbackRunner(execChan)
    for {
		conn, err := ln.Accept()
		if err != nil {
			// handle error
			continue
		}
		//fmt.Println(conn)
		go handleConnection(conn, cs.root, execChan, cs.data)
    }
    return nil
}

func handleConnection(conn net.Conn, currNode *CimNode, execChan chan ExecData, data interface{}) {

    buf := make([]byte, 1024*1024)
    rootNode := currNode
    n, err := conn.Read(buf)
    //fmt.Println("Got login message :", buf[:n])
    if bytes.Equal(buf[:n], gen_login_msg()) == false {
		fmt.Println("Wrong login message")
		conn.Close()
		return
    }
    n, err = conn.Write(gen_login_response())
    for err == nil {
		n, err = conn.Read(buf)
		if n == 0 {
			continue
		}
		//fmt.Println("n =", n, "buf =", buf)
		cmdline := get_cmd_from_msg(buf, n)
		if strings.Trim(cmdline, " \n\t\r") == "" {
			n, err = conn.Write(gen_output_msg_from_output("Empty command!"))
			continue
		}
		//fmt.Println("Got the command : ", cmdline)
		cmd := strings.Fields(cmdline)
		if is_builtin_cmd(cmd[0]) == true {
			n, err = conn.Write(gen_output_msg_from_output(run_builtin_cmd(rootNode, &currNode, cmd[0], cmd[1:]...)))
			continue
		}
		cb, ok := currNode.Callbacks[cmd[0]]
		if ok == false {
			n, err = conn.Write(gen_output_msg_from_output("Invalid command"))
			continue
		}
		outchan := make(chan string)
		ed := ExecData{cb, cmd[0], cmd[1:], data, outchan}
		execChan <- ed
		out := <-outchan
		//fmt.Println("Sending output :", out)
		n, err = conn.Write(gen_output_msg_from_output(out))
		
    }
    conn.Close()
}

/*
type ExecData struct {
    Callback CBfunc
    Cmd string
    Args []string
    Data interface{}
    RetChan chan string
}

*/

func callbackRunner(execChan chan ExecData) {
    for execpkt := range execChan {
		execpkt.RetChan <- execpkt.Callback(execpkt.Data, execpkt.Cmd, execpkt.Args...)
    }
}

/*
type CimNode struct {
    IsLeaf bool
    Name string
    Path string
    Children []*CimNode
    Callbacks map[string]CBfunc
    Parent *CimNode
}

*/
func run_builtin_cmd(rootNode *CimNode, currNodePtr **CimNode, cmd string, args ...string) string {
    currNode := *currNodePtr
    if cmd == "ls" {
		out := ""
		for i:=0; i<len(currNode.Children); i++ {
			if currNode.Children[i] != nil {
				out += (currNode.Children[i].Name + "\n")
			}
		}
		for cbname,_ := range currNode.Callbacks {
			out += ( cbname + "*\n")
		}
		return out
    } else if cmd == "pwd" {
		return currNode.Path
    } else if cmd == "cd" {
		if len(args) == 0 {
			*currNodePtr = rootNode
			return ""
		} else if strings.Contains(args[0], "/") == false {
			if args[0] == ".." {
				if currNode.Name == "/" {
					return ""
				} else {
					*currNodePtr = currNode.Parent
					return ""
				}
			} else {
				
				for i:=0; i<len(currNode.Children); i++ {
					if currNode.Children[i] != nil {
						if currNode.Children[i].Name == args[0] { *currNodePtr = currNode.Children[i]; return "" }
					}
				}
				return "There is no directory called " + args[0]
			}
		}
    }
    return ""
}

func is_builtin_cmd(cmd string) bool {
    if cmd == "ls" || cmd == "cd" || cmd == "pwd" {
		return true
    }
    return false
}

func NewCimConnection(host, simInst, connId string) (*CimConnection, error) {
    cc := new(CimConnection)
    if host == "" {
		host = "127.0.0.1"
    }
    cc.host = host
    cc.simInst = simInst
    var ccfg *qcfg.CfgBlock = qcfg.NewCfg(connId, cfgFile, false)
    var port = ccfg.Str("ports-lst", "ports", simInst, "")
    if port == "" {
		return nil, errors.New("Invalid sim : " + simInst)
    }
    cc.port = port
    err := errors.New("")
    cc.tcpAddr, err = net.ResolveTCPAddr("tcp4", cc.host + ":" + cc.port)
    if err != nil {
		return nil, err
    }
    cc.tcpConn, err = net.DialTCP("tcp4", nil, cc.tcpAddr)
    if err != nil {
		return nil, err
    }
    err1 := cc.tcpConn.SetKeepAlive(true)
    err2 := cc.tcpConn.SetNoDelay(true)
    if err1 != nil || err2 !=nil {
		cc.tcpConn.Close()
		return nil, errors.New("Cannot set KeepAlive or NoDelay")
    }
    _, err = cc.tcpConn.Write(gen_login_msg())
    if err != nil {
		cc.tcpConn.Close()
		return nil, errors.New("Cannot send login message to sim")
    }
    buf := make([]byte, 1024)
    _, err = cc.tcpConn.Read(buf)
    if err != nil {
		cc.tcpConn.Close()
		return nil, errors.New("Cannot read login msg response from sim")
    }
    //fmt.Println("Login response :", buf[:n])
    cc.resp = make([]byte, responseMaxLength)
    return cc, nil
}

func gen_cmd_msg(cmd string) []byte {
    var header = []byte{0, 1 ,11, 0}
    var msg = make([]byte, 5+len(cmd))
    var i = 0
    for i=0; i<4; i++ {
        msg[i] = header[i]
    }
    for i=0; i<len(cmd); i++ {
		msg[i+4] = byte(cmd[i])
    }
    msg[4+len(cmd)] = 0
    x := len(cmd) + 1
    msg[2] = byte(x & 255) //lower byte of x                                                                                                                                                                       
    msg[3] = byte(x >> 8)  //higher byte of x 
    return msg
}

func get_cmd_from_msg(msg []byte, length int) string {
    if length < 6 {
		return ""
    }
    return string(msg[4:length-1])
}

func gen_login_msg() []byte {
    //var login_msg = []byte{0, 3, 24, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 1, 0, 0, 0, 0, 0, 0}
    var login_msg = []byte{0, 3, 12, 0, 1, 0, 0, 0, 0, 0, 0, 0, 8, 1, 0, 0}
    return login_msg
}

func gen_login_response() []byte {
    var resp = []byte{0, 3, 12, 0, 1, 0, 188, 55, 0, 0, 0, 0, 1, 1, 123, 227}
    return resp
}

func gen_output_msg_from_output(output string) []byte {
    // 0 2 21 0 83 116 114 97 116 101 103 121 32 105 115 32 114 117 110 110 105 110 103 10 0
    n := len(output)
    buf := make([]byte, n+6)
    buf[0] = 0
    buf[1] = 2
    buf[2] = byte( (n+2) & 255 )
    buf[3] = byte( (n+2) >> 8 )
    for i:=0; i<n; i++ {
		buf[4+i] = byte(output[i])
    }
    buf[n+4] = 10 //The last two bytes are part of the data, so length = n+2
    buf[n+1+4] = 0
    //fmt.Println("Output buf = ", buf)
    return buf
}

func extract_output(buf []byte, length int) string {
    //fmt.Println("command_output : ", buf[:length])
    if length < 6 {
		return ""
    }
    last_newline_index := bytes.LastIndex(buf[:length], []byte{10})
    if last_newline_index == -1 {
		return ""

    }
    return string(buf[4:last_newline_index])
}

func (cc *CimConnection) Close() error {
	cc.resp = nil
	return cc.tcpConn.Close()
}

func (cc *CimConnection) RunCommand(cmd string) (string, error) {
    buf := gen_cmd_msg(cmd)
    _, err := cc.tcpConn.Write(buf)
    if err != nil {
		return "", err
    }
    
    n, err := cc.tcpConn.Read(cc.resp)
    if err != nil {
		return "", err
    }
    if bytes.HasPrefix(cc.resp[:n], []byte{0, 4, 2, 0, 0, 0}) == true {
		//fmt.Println(cc.resp[:n])
		n, err = cc.tcpConn.Read(cc.resp)
		if err != nil {
			return "", err
		}
    }
    //fmt.Println(cc.resp[:n])
    return extract_output(cc.resp, n), nil
    
}
