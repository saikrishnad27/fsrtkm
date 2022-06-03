package main

import (
	pb "AOSProject2/AOSProject_2"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"
)

type Domain struct {
	low  uint64
	mid  uint64
	high uint64
}
type State struct {
	pvalue *uint64
	fvalue *uint64
}

// to store reader and writers
type RToken struct {
	Writer string
	Reader []string
}

// to store in the readtokenlist and this is used while reading
type ReadTokenList struct {
	id      string
	name    string
	low     uint64
	mid     uint64
	high    uint64
	pval    string
	fval    string
	counter uint64
}
type Token struct {
	key     string
	name    string
	domain  Domain
	state   State
	writer  string
	reader  []string
	counter uint64
	mutex   sync.RWMutex
}

var lock sync.Mutex
var tkns map[string]Token

func init() {
	//initalizing the tokens
	tkns = make(map[string]Token)
	yfile, err := ioutil.ReadFile("token.yaml")
	if err != nil {

		log.Fatal(err)
	}

	data := make(map[string]RToken)

	err2 := yaml.Unmarshal(yfile, &data)

	if err2 != nil {

		log.Fatal(err2)
	}
	for k, v := range data {
		c := Token{key: k, mutex: sync.RWMutex{}, writer: v.Writer, reader: v.Reader}
		tkns[k] = c
	}
	fmt.Println("token map:", tkns)

}

type TokenServer struct {
	pb.UnimplementedTokenServer
}

// generates the hash value based on  name and nonce
func Hash(name string, nonce uint64) uint64 {
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%s %d", name, nonce)))
	return binary.BigEndian.Uint64(hasher.Sum(nil))
}

// display the token data
func displayTokenData(a Token) {
	fmt.Println("Token Data value:")
	fmt.Println("Token ID:", a.key)
	fmt.Println("Token Name:", a.name)
	fmt.Println("Token Domain low:", a.domain.low)
	fmt.Println("Token Domain mid:", a.domain.mid)
	fmt.Println("Token Domain high:", a.domain.high)
	if a.state.pvalue != nil {
		fmt.Println("Token state partial value:", *a.state.pvalue)
	} else {
		fmt.Println("Token state partial value: null")
	}
	if a.state.fvalue != nil {
		fmt.Println("Token state final value:", *a.state.fvalue)
	} else {
		fmt.Println("Token state final value: null")
	}
}

// displays all the ids related to tokens
func displayId() {
	TokenIds := reflect.ValueOf(tkns).MapKeys()
	fmt.Println("The list of TokenIds:")
	fmt.Println(TokenIds)
}

// gives the minimum x value based on the intervals
func argmin_x(name string, n1 uint64, n2 uint64) uint64 {
	k := Hash(name, n1)
	l := n1
	for i := 0; n1 < n2-1; i++ {
		n1 = n1 + 1
		temp := Hash(name, n1)
		if temp < k {
			k = temp
			l = n1
		}
	}
	return l
}

//just reading of the atomic register is done and here the value is not at all comitted
func (s *TokenServer) ReadARToken(ctx context.Context, in *pb.ARRead) (*pb.ARReadRes, error) {
	fmt.Println("reading the request from reader node")
	ab, isPresent := tkns[in.GetId()]
	if len(ab.name) != 0 && isPresent {
		//applying readlock
		ab.mutex.RLock()
		name := ab.name
		l := ab.domain.low
		m := ab.domain.mid
		h := ab.domain.high
		a := argmin_x(name, m, h)
		// fmt.Println("the final value in this block is", a)
		// fmt.Println("the final value in this block WITH PARTIAL is", Hash(name, *ab.state.pvalue))
		// fmt.Println("the final value in this block WITH final is", Hash(name, a))
		a1 := strconv.FormatUint(*ab.state.pvalue, 10)
		if Hash(name, *ab.state.pvalue) <= Hash(name, a) {
			ab.state.fvalue = ab.state.pvalue
		} else {
			ab.state.fvalue = &a
		}
		a2 := strconv.FormatUint(*ab.state.fvalue, 10)
		sta := ab.counter
		ab.mutex.RUnlock()
		//removing the value
		//tkns[in.GetId()] = ab
		displayTokenData(ab)
		fmt.Println("the writing of final value is not yet comitted and the actual value is")
		displayTokenData(tkns[in.GetId()])
		return &pb.ARReadRes{Id: in.GetId(), Name: name, Low: l, Mid: m,
			High: h, Pval: a1, Fval: a2, Counter: sta, Msg: "success"}, nil
	} else {
		return &pb.ARReadRes{Msg: "absent"}, nil
	}
}

//performing the write ar token operation and here the token is commited but it can be revert back when it hasn't done in most of the servers
func (s *TokenServer) WriteARToken(ctx context.Context, in *pb.ARWrite) (*pb.ARWriteRes, error) {
	fmt.Println("writing the update from writer node")
	ab, isPresent := tkns[in.GetId()]
	if isPresent {
		//checking whether write is related to read or write
		if in.GetFlag() == "write" {
			// for write checking whether the counter value is more than previous or not
			if in.GetCounter() < ab.counter {
				return &pb.ARWriteRes{Msg: "failed"}, nil
			}
			fmt.Println("entered writing block related to writing")
			ab.mutex.Lock()
			ab.state.fvalue = nil

		} else {
			//sending this response because while performing this write which is related to read a actual write has happen
			//inorder to preserve the policy that the write of w does not precede the write of v as given in question
			if in.GetCounter() != ab.counter {
				return &pb.ARWriteRes{Msg: "changed"}, nil
			}
			fmt.Println("entered writing block related to reading")
			ab.mutex.RLock()
			n1, err := strconv.ParseUint(in.GetFval(), 10, 64)
			if err != nil {
				ab.mutex.RUnlock()
				return &pb.ARWriteRes{Msg: "failed"}, nil
			}
			ab.state.fvalue = &n1
		}
		ab.name = in.GetName()
		ab.domain.low = in.GetLow()
		ab.domain.mid = in.GetMid()
		ab.domain.high = in.GetHigh()
		ab.counter = in.GetCounter()
		a1 := in.GetPval()
		n, err := strconv.ParseUint(a1, 10, 64)
		if err != nil {
			if in.GetFlag() == "write" {
				ab.mutex.Unlock()
			} else {
				ab.mutex.RUnlock()
			}
			return &pb.ARWriteRes{Msg: "failed"}, nil
		}
		ab.state.pvalue = &n
		// making sure to remove the locks properly
		if in.GetFlag() == "write" {
			ab.mutex.Unlock()
		} else {
			ab.mutex.RUnlock()
		}
		tkns[in.GetId()] = ab
		displayTokenData(ab)
		return &pb.ARWriteRes{Msg: "success"}, nil

	} else {
		fmt.Println("No Token present for given Id")
		return &pb.ARWriteRes{Msg: "failed"}, nil
	}
}

// when you want to perform the read this process is invoked and this function collects all the values related
//to read and it stores in a list and reads the one with highest counter(timestamp)
func ReadingProcess(ab Token, cd1 chan<- uint64, tnum uint64) {
	fmt.Println("token Reading process started from other nodes")
	//if reading didn't perform successfull making sure the value is rolled back
	bc, _ := tkns[ab.key]
	name := ab.name
	m := ab.domain.mid
	high := ab.domain.high
	a := argmin_x(name, m, high)
	cat := -1
	//fmt.Println("the value between mid to high is", a)
	//updating the read based on hash values after comparing with partial value
	var h1 uint64
	if Hash(name, *ab.state.pvalue) <= Hash(name, a) {
		ab.state.fvalue = ab.state.pvalue
		h1 = *ab.state.pvalue
	} else {
		ab.state.fvalue = &a
		h1 = a
	}
	h := len(ab.reader) / 2
	var readlist []ReadTokenList
	h2 := strconv.FormatUint(h1, 10)
	h3 := strconv.FormatUint(*ab.state.pvalue, 10)
	readlist = append(readlist, ReadTokenList{id: ab.key, name: ab.name, low: ab.domain.low,
		mid: ab.domain.mid, high: ab.domain.high, fval: h2, pval: h3, counter: ab.counter})
	c1 := 1
	sa := 0
	var a12 uint64
	for i := range ab.reader {
		fmt.Println("the value of i is", i)
		if tnum == uint64(i) {
			// as this node is choosen node and we already performed the operation we are not making grpc call because it will throw a error
			continue
		}
		fmt.Println("the read request is being sent to")
		fmt.Printf("%s\n", ab.reader[i])
		address := ab.reader[i]
		conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("did not connect: %v", err)
		}
		defer conn.Close()
		c := pb.NewTokenClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		r, err := c.ReadARToken(ctx, &pb.ARRead{Id: ab.key})
		if err != nil {
			fmt.Println("unable to connect to this server", ab.reader[i])
			continue
		}
		if r.GetMsg() == "absent" {
			// this message is returned when this particular node is not present
			continue
		} else {
			fmt.Println("entered readlist appending block")
			//appending to the list inorder to find the recently updated token
			readlist = append(readlist, ReadTokenList{id: r.Id, name: r.Name, low: r.Low,
				mid: r.Mid, high: r.High, fval: r.Fval, pval: r.Pval, counter: r.Counter})
			fmt.Println(readlist)
			c1 = c1 + 1
		}
		fmt.Println("checking the acknowledgement")
		if c1 >= h+1 {
			//making sure the latest counter value is read
			if cat < int(r.GetCounter()) {
				fmt.Println("acknowledgement value has been more than half the number of nodes")
				temp := readlist[0]
				for _, element := range readlist {
					if element.counter > temp.counter {
						temp = element
						cat = int(element.counter)
					}
				}
				//fmt.Println("the highest counter value", temp.counter)
				str := ReadTokenList{id: temp.id, name: temp.name,
					low: temp.low, mid: temp.mid, high: temp.high,
					pval: temp.pval, fval: temp.fval, counter: temp.counter}
				cd := make(chan uint64)
				Reading := "true"
				go WritingProcess(cd, str, Reading, tnum)
				a12 = <-cd
				fmt.Println("the received data is", a12)
				close(cd)
				// ki := -9999999
				// if a12 == uint64(ki) {
				// 	sa = sa + 1
				// }
				sa = sa + 1
			}
		}
	}
	if sa == 0 {
		tkns[ab.key] = bc
		displayTokenData(tkns[ab.key])
		ki := -9999999
		cd1 <- uint64(ki)
	} else {
		fmt.Println("sending data over channel ", a12)
		cd1 <- a12
	}
}

//to make sure that if write didn't occure properly in n/2+1 nodes rolling it back to previous state
func (s *TokenServer) RollbackARToken(ctx context.Context, in *pb.ARRollbackReq) (*pb.ARWriteRes, error) {
	fmt.Println("discarding the write change to this particular node")
	ab, isPresent := tkns[in.GetId()]
	if isPresent {
		ab.mutex.Lock()
		ab.name = in.GetName()
		ab.counter = in.GetCounter()
		ab.domain.low = in.GetLow()
		ab.domain.high = in.GetHigh()
		ab.domain.mid = in.GetMid()
		if in.GetFval() == "NULL" {
			ab.state.fvalue = nil
		} else {
			a, _ := strconv.ParseUint(in.GetFval(), 10, 64)
			ab.state.fvalue = &a
		}
		if in.GetPval() == "NULL" {
			ab.state.pvalue = nil
		} else {
			b, _ := strconv.ParseUint(in.GetPval(), 10, 64)
			ab.state.fvalue = &b
		}
		ab.mutex.Unlock()
		displayTokenData(ab)
		return &pb.ARWriteRes{Msg: "success"}, nil

	}
	return &pb.ARWriteRes{Msg: "fail"}, nil
}

// this method is called to roll back the servers with values which aren't properly comitted
func discardwritingprocess(ab Token, cd chan<- string, writer []string) {
	var f1, p1 string
	if ab.state.fvalue != nil {
		f1 = strconv.FormatUint(*ab.state.fvalue, 10)
	} else {
		f1 = "NULL"
	}
	if ab.state.pvalue != nil {
		p1 = strconv.FormatUint(*ab.state.pvalue, 10)
	} else {
		p1 = "NULL"
	}
	k := 0
	for i := range writer {
		address := writer[i]
		fmt.Println("discard changes are being sent to this particular server", writer[i])
		conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("did not connect: %v", err)

		}
		defer conn.Close()
		c := pb.NewTokenClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		r, err := c.RollbackARToken(ctx, &pb.ARRollbackReq{Id: ab.key, Name: ab.name, Low: ab.domain.low,
			High: ab.domain.high, Mid: ab.domain.mid, Fval: f1, Pval: p1, Counter: ab.counter})
		if err != nil {
			//roll back failed
			fmt.Println("write didn't perform on", ab.reader[i])
			continue
		}
		if r.GetMsg() == "success" {
			//making sure that roll back done successfully
			k = k + 1
		}

	}
	if k == len(writer) {
		fmt.Println("discarded write performed successfully")
		cd <- "go"
	} else {
		fmt.Println("discarded write performed partially")
		cd <- "go"
	}
}

// function for writing
func WritingProcess(cd chan<- uint64, in ReadTokenList, reading string, tnum uint64) {
	fmt.Println("token writing process started for other nodes")
	//implementing fail silent behavior for this particular token id=124
	if in.id == "124" {
		fmt.Println("fail silent behavior is being performed")
		go func() {}()
	} else {
		var a []string
		ab, _ := tkns[in.id]
		//preserving the copy incase of  roll back
		bc := ab
		ab.mutex.Lock()
		ab.name = in.name
		ab.domain.low = in.low
		ab.domain.mid = in.mid
		ab.domain.high = in.high
		l := ab.domain.low
		m := ab.domain.mid
		name := ab.name
		var a1, number, number1 uint64
		//if its writing make sure that  partial value and final value are nil
		if reading == "false" {
			fmt.Println("argmin function called")
			a1 = argmin_x(name, l, m)
			ab.state.pvalue = &a1
			ab.state.fvalue = nil
			ab.counter = in.counter + 1
		} else {
			//if its reading update the partial and final value
			ab.counter = in.counter
			x1 := in.pval
			x2 := in.fval
			n3, err := strconv.ParseUint(x1, 10, 64)
			if err == nil {
				ab.state.pvalue = &n3
			}
			n4, err := strconv.ParseUint(x2, 10, 64)
			if err == nil {
				ab.state.fvalue = &n4
			}
			number = n3
			number1 = n4

		}
		c1 := 0
		h4 := len(ab.reader) - 1
		fmt.Println("the value of h4 is", h4)
		//knowing the half of the nodes in which the copy is there
		h := len(ab.reader) / 2
		k := 0
		sa := 0
		sp := 0
		for i := range ab.reader {
			k = 0
			if reading == "false" {
				//checking whether the reader and particular writer are same
				if ab.writer == ab.reader[i] {
					c1 = c1 + 1
					k = 1
					sp = sp + 1
				}
			}
			if reading == "true" {
				//checking which node is the same node
				if tnum == uint64(i) {
					c1 = c1 + 1
					k = 1
				}
			}
			if k != 1 {
				fmt.Println("the write request is being sent to")
				fmt.Printf("%s\n", ab.reader[i])
				address := ab.reader[i]
				conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					log.Fatalf("did not connect: %v", err)

				}
				defer conn.Close()
				c := pb.NewTokenClient(conn)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				var e1, e2, flag string
				if reading == "false" {
					e1 = strconv.FormatUint(a1, 10)
					e2 = "NULL"
					flag = "write"
				} else {
					e1 = strconv.FormatUint(number, 10)
					e2 = strconv.FormatUint(number1, 10)
					flag = "read"
				}
				r, err := c.WriteARToken(ctx, &pb.ARWrite{Id: ab.key, Name: ab.name,
					Low: ab.domain.low, Mid: ab.domain.mid,
					High: ab.domain.high, Pval: e1, Fval: e2, Counter: ab.counter, Flag: flag})
				if err != nil {
					fmt.Println("write didn't perform on", ab.reader[i])
					//continue
					//log.Fatalf("write operation failed: %v", ab.reader[i])
				}
				if r.GetMsg() == "changed" {
					// a write has occured and the state is changed and
					//this is particularly related to read request
					sa = -9
					break
				}
				if r.GetMsg() == "success" {
					c1 = c1 + 1
					a = append(a, ab.reader[i])
				}
			}
			if reading == "false" {
				if i == h4 && sp == 0 {
					c1 = c1 + 1
				}
			}
			fmt.Println("checking the acknowledgement")
			if c1 == h+1 {
				fmt.Println("acknowledgement value has been more than the number of process")
				ab.mutex.Unlock()
				tkns[in.id] = ab
				if reading == "false" {
					cd <- a1
					sa = sa + 1
				} else {
					fmt.Println("sending data over channel 2")
					cd <- number1
					sa = sa + 1
				}

			}
		}
		if sa == -9 {
			ab.mutex.Unlock()
			ki := -888
			cd <- uint64(ki)
		} else if sa == 0 {
			ab.mutex.Unlock()
			tkns[in.id] = bc
			ak := make(chan string)
			go discardwritingprocess(bc, ak, a)
			str := <-ak
			if str == "go" {
				ki := -9999999
				cd <- uint64(ki)
			}
		}
	}
}
func (s *TokenServer) WriteToken(ctx context.Context, in *pb.Wkey) (*pb.WRResponse, error) {
	fmt.Println("Received the write request:", in)
	_, isPresent := tkns[in.GetKey().GetId()]
	// checks whether given token id is present or not
	if isPresent {
		fmt.Println("TOKEN WRITING BLOCK")
		cd := make(chan uint64)
		id1 := in.GetKey().GetId()
		str := ReadTokenList{id: id1, name: in.GetName(),
			low: in.GetLow(), mid: in.GetMid(), high: in.GetHigh()}
		Reading := "false"
		go WritingProcess(cd, str, Reading, 0)
		a := <-cd
		close(cd)
		fmt.Println("channel closed")
		ki := -9999999
		if a == uint64(ki) {
			return &pb.WRResponse{Err: "write operation failed due to unable to write in n/2 +1 nodes"}, nil
		}
		abc := tkns[in.GetKey().GetId()]
		displayTokenData(abc)
		displayId()
		return &pb.WRResponse{Res: a}, nil
	} else {
		fmt.Println("No Token present for given Id")
		return &pb.WRResponse{Err: "failed"}, nil
	}
}

func readprocess(ab Token, in *pb.Key, cd chan<- uint64) {
	cd1 := make(chan uint64)
	var a uint64
	// var s string
	//writing a infinite loop just to make sure that correct value is read
	for {
		go ReadingProcess(ab, cd1, in.GetNum())
		a = <-cd1
		close(cd1)
		ki := -9999999
		fmt.Println("the value is", a)
		if a == uint64(ki) {
			break
		}
		if a != uint64(888) {
			break
		}
	}
	cd <- a

}

// performing the read operation
func (s *TokenServer) ReadToken(ctx context.Context, in *pb.Key) (*pb.WRResponse, error) {
	fmt.Println("Received the Read request:", in)
	ab, isPresent := tkns[in.GetId()]
	// checking whether the id is present and checking whether name is there or not
	if len(ab.name) != 0 && isPresent {
		fmt.Println("TOKEN Reading BLOCK")
		cd1 := make(chan uint64)
		var a uint64
		go readprocess(ab, in, cd1)
		a = <-cd1
		close(cd1)
		ki := -9999999
		if a == uint64(ki) {
			return &pb.WRResponse{Err: "read operation failed due to unable to write in n/2 +1 nodes"}, nil
		}
		fmt.Println("the received value is", a)
		abc := tkns[in.GetId()]
		displayTokenData(abc)
		displayId()
		return &pb.WRResponse{Res: a}, nil
	} else {
		fmt.Println("No Token present for given Id or the name is not initalized yet")
		k := "failed"
		return &pb.WRResponse{Err: k}, nil
	}
}

//rolling back not fully commited drop
func (s *TokenServer) RollbackDropARToken(ctx context.Context, in *pb.ARRollbackReq) (*pb.ARWriteRes, error) {
	fmt.Println("discarding the drop change to this particular node")
	_, isPresent := tkns[in.GetId()]
	if isPresent {
		return &pb.ARWriteRes{Msg: "success"}, nil
	} else {
		ab := Token{}
		ab.key = in.GetId()
		ab.name = in.GetName()
		ab.counter = in.GetCounter()
		ab.domain.low = in.GetLow()
		ab.domain.high = in.GetHigh()
		ab.domain.mid = in.GetMid()
		if in.GetFval() == "NULL" {
			ab.state.fvalue = nil
		} else {
			a, _ := strconv.ParseUint(in.GetFval(), 10, 64)
			ab.state.fvalue = &a
		}
		if in.GetPval() == "NULL" {
			ab.state.pvalue = nil
		} else {
			b, _ := strconv.ParseUint(in.GetPval(), 10, 64)
			ab.state.fvalue = &b
		}
		strArray := strings.Fields(in.GetReader())
		ab.reader = strArray
		ab.writer = in.GetWriter()
		tkns[ab.key] = ab
		displayTokenData(ab)
		return &pb.ARWriteRes{Msg: "success"}, nil
	}

}

// deleting the token based on id
func (s *TokenServer) DropToken(ctx context.Context, in *pb.Key) (*pb.DResponse, error) {
	fmt.Println("Received the drop request:", in)
	tok, isPresent := tkns[in.GetId()]
	if isPresent {
		var b []string
		tok.mutex.Lock()
		if in.GetFlag() == "main" {
			arr := tok.reader
			ack := 1
			for i := range arr {
				if tok.writer == arr[i] {
					continue
				}
				address := arr[i]
				conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					log.Fatalf("did not connect: %v", err)

				}
				defer conn.Close()
				c := pb.NewTokenClient(conn)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_, err1 := c.DropToken(ctx, &pb.Key{Id: in.GetId(),
					Flag: "dup"})
				if err1 != nil {
					continue
				}
				b = append(b, arr[i])
				ack = ack + 1
			} //checking if this operation is commited in more than half of nodes in it.
			if ack > len(arr)/2 {
				delete(tkns, in.GetId())
				fmt.Println("Deletion of the given Id was successfully")
				displayId()
				tok.mutex.Unlock()
			} else {
				for i := range b {
					address := b[i]
					conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
					if err != nil {
						log.Fatalf("did not connect: %v", err)
					}
					defer conn.Close()
					c := pb.NewTokenClient(conn)
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					var f1, p1 string
					if tok.state.fvalue != nil {
						f1 = strconv.FormatUint(*tok.state.fvalue, 10)
					} else {
						f1 = "NULL"
					}
					if tok.state.pvalue != nil {
						p1 = strconv.FormatUint(*tok.state.pvalue, 10)
					} else {
						p1 = "NULL"
					}
					str2 := strings.Join(tok.reader, " ")
					_, err1 := c.RollbackDropARToken(ctx, &pb.ARRollbackReq{Id: tok.key, Name: tok.name,
						Low: tok.domain.low, Mid: tok.domain.mid, High: tok.domain.high, Pval: p1, Fval: f1, Counter: tok.counter,
						Writer: tok.writer, Reader: str2})
					if err1 != nil {
						log.Fatalf("Drop Operation failed: %v", err1)
					}
				}
				fmt.Println("Deletion operation didn't commited")
				tok.mutex.Unlock()
			}
		} else {
			delete(tkns, in.GetId())
			fmt.Println("Deletion of the given Id was successfully")
			//displayTokenData(a)
			displayId()
			tok.mutex.Unlock()
		}
	} else {
		fmt.Println("No Token present for given Id")
	}
	return &pb.DResponse{}, nil
}

// function performing the create token
func (s *TokenServer) CreateToken(ctx context.Context, in *pb.Key) (*pb.CResponse, error) {
	fmt.Println("Received the create request:", in)
	_, isPresent := tkns[in.GetId()]
	// checking whether the id is already exsists or not
	if isPresent {
		fmt.Println("Id already exsisting")
		return &pb.CResponse{Res: "failure"}, nil
	} else {
		lock.Lock()
		strArray := strings.Fields(in.GetNodes())
		c := Token{key: in.GetId(), mutex: sync.RWMutex{}, writer: strArray[in.GetNum()], reader: strArray}
		ack := 1
		if in.GetFlag() == "main" {
			var b []string
			for i := range strArray {
				if in.GetNum() == uint64(i) {
					continue
				}
				address := strArray[i]
				conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					log.Fatalf("did not connect: %v", err)

				}
				defer conn.Close()
				c := pb.NewTokenClient(conn)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				r, err := c.CreateToken(ctx, &pb.Key{Id: in.GetId(), Num: in.GetNum(),
					Flag: "dup", Nodes: in.GetNodes()})
				if err != nil {
					continue
				}
				if r.GetRes() == "success" {
					b = append(b, strArray[i])
					ack = ack + 1
				}
			} //checking if this operation is commited in more than half of nodes in it.
			if ack > len(strArray)/2 {
				fmt.Println("entered this block successfully")
				tkns[in.GetId()] = c
				displayTokenData(c)
				fmt.Println("Token Id created successfully")
				displayId()
				lock.Unlock()
				return &pb.CResponse{Res: "success"}, nil
			} else {
				for i := range b {
					address := b[i]
					conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
					if err != nil {
						log.Fatalf("did not connect: %v", err)
					}
					defer conn.Close()
					c := pb.NewTokenClient(conn)
					ctx, cancel := context.WithTimeout(context.Background(), time.Second)
					defer cancel()
					_, err1 := c.DropToken(ctx, &pb.Key{Id: os.Args[3], Flag: "dup"})
					if err1 != nil {
						log.Fatalf("Drop Operation failed: %v", err1)
					}
				}
				lock.Unlock()
				return &pb.CResponse{Res: "failed"}, nil
			}
		} else {
			tkns[in.GetId()] = c
			displayTokenData(c)
			fmt.Println("Token Id created successfully")
			displayId()
			lock.Unlock()
			return &pb.CResponse{Res: "success"}, nil
		}
	}
}

func main() {
	if os.Args[1] == "tokenserver" {
		// checking whether port number is valid or not
		_, err := strconv.Atoi(os.Args[3])
		if err != nil {
			fmt.Println("Invalid port value")
		} else {
			// establishing the connection
			lis, err := net.Listen("tcp", ":"+os.Args[3])
			if err != nil {
				log.Fatalf("failed to listen: %v", err)
			}
			s := grpc.NewServer()
			pb.RegisterTokenServer(s, &TokenServer{})
			//to perform a graceful stop
			sigchan := make(chan os.Signal)
			signal.Notify(sigchan, os.Interrupt, syscall.SIGTERM)
			wg := sync.WaitGroup{}
			wg.Add(1)
			go func() {
				st := <-sigchan
				fmt.Println("got signal", st)
				s.GracefulStop()
				fmt.Println("server gracefully stopped")
				wg.Done()
			}()
			fmt.Println("serverlistening at", lis.Addr())
			// serve starts listening
			if err := s.Serve(lis); err != nil {
				log.Fatalf("failed to serve: %v", err)
			}
			wg.Wait()
		}
	} else {
		fmt.Println("Invalid input")
	}
}
