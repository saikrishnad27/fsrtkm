package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	pb "AOSProject2/AOSProject_2"
	"io/ioutil"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"

	// "reflect"
	"strings"
)

type Output struct {
	v     int
	host  string
	nodes []string
}

// to store the input output
type Token struct {
	Writer string
	Reader []string
}

var replica []string
var tkns map[string]Token

func init() {
	//initalization of tokens during the starting of client itself
	tkns = make(map[string]Token)
	yfile, err := ioutil.ReadFile("token.yaml")

	if err != nil {
		log.Fatal(err)
	}

	data := make(map[string]Token)

	err2 := yaml.Unmarshal(yfile, &data)

	if err2 != nil {
		log.Fatal(err2)
	}
	print("the data is", data)
	// making it as token struct and storing it in a map
	for k, v := range data {
		c := Token{Writer: v.Writer, Reader: v.Reader}
		tkns[k] = c
	}
	fmt.Println("token map:", tkns)
	//initalizing replica for creation of string
	replica = []string{"localhost:8000", "localhost:8001", "localhost:8002", "localhost:8003"}

}

// validating whether input is correct or not and returning output object
func check_input(s string) Output {
	v := 0
	var c Output
	c = Output{v: v}
	words := strings.Fields(s)
	if words[0] == "tokenclient" {
		if (words[1] == "-create") || words[1] == "-read" {
			//validating create and read requests
			if len(words) == 4 && len(words[3]) != 0 {
				if words[1] == "-read" {
					v = 2
					for k1, v1 := range tkns {
						fmt.Println("the value of id is", k1)
						if k1 == words[3] {
							//sending the result required for read request
							c = Output{v: v, host: v1.Reader[0], nodes: v1.Reader}
							return c
						}
					}
				} else {
					fmt.Println("entered this block")
					_, isPresent := tkns[words[3]]
					if isPresent {
						fmt.Println("token already exists")
						return c
					} else {
						//sending the result required for create request
						c = Output{v: 1, nodes: replica}
						return c
					}
				}
			}
		} else if words[1] == "-drop" {
			//sending the result required for drop request
			_, isPresent := tkns[words[2]]
			if isPresent {
				for k1, v1 := range tkns {
					fmt.Println("the value of id is", k1)
					if k1 == words[2] {
						c = Output{v: 3, host: v1.Writer, nodes: v1.Reader}
						return c
					}
				}
			}

		} else if len(words) == 12 && len(words[3]) != 0 {
			//sending the result required for write request
			k := len(words[5])
			l, err := strconv.Atoi(words[7])
			m, err1 := strconv.Atoi(words[9])
			h, err2 := strconv.Atoi(words[11])
			if (err == nil && err1 == nil) && err2 == nil {
				if (l <= m) && (m < h) {
					if k != 0 {
						v = 4
						for k1, v1 := range tkns {
							fmt.Println("the value of id is", k1)
							if k1 == words[3] {
								c = Output{v: v, host: v1.Writer, nodes: v1.Reader}
								return c
							}
						}

					}
				}

			}
		}
	}
	return c
}

func main() {
	justString := strings.Join(os.Args[1:], " ")
	// storing the input in slice
	data := check_input(justString) // to check whether the given input is crct or not
	if data.v == 0 {
		fmt.Println("INVALID INPUT")
	} else {
		if data.v == 1 {
			//starting the create request
			for k1, v1 := range data.nodes {
				address := v1
				print("the address is", v1)
				// sending it to this particular node
				conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					log.Fatalf("did not connect: %v", err)
				}
				defer conn.Close()
				c := pb.NewTokenClient(conn)
				// ctx := context.Background()
				// ctx, cancel := context.WithCancel(ctx)
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				str2 := strings.Join(data.nodes, " ")
				r, err := c.CreateToken(ctx, &pb.Key{Id: os.Args[4], Num: uint64(k1), Nodes: str2, Flag: "main"})
				if err != nil {
					//when it fails trying to perform on the second server and giving other servers as replicas
					fmt.Println("unable to connect to this particular creation node", v1)
					continue
					//log.Fatalf("Read operation failed: %v", err)
				}
				fmt.Println("Response is : ", r.GetRes())
				break
			}
		} else if data.v == 2 {
			// sending read token request
			for k1, v1 := range data.nodes {
				// checking one after other node instead of going random because random function sometimes repeatedly generate same number
				address := v1
				print("the address is", v1)
				//establishing the connection
				conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
				if err != nil {
					log.Fatalf("did not connect: %v", err)
				}
				defer conn.Close()
				c := pb.NewTokenClient(conn)
				// ctx := context.Background()
				// ctx, cancel := context.WithCancel(ctx)
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				r, err := c.ReadToken(ctx, &pb.Key{Id: os.Args[4], Num: uint64(k1)})
				if err != nil {
					fmt.Println("unable to connect to this particular read node", v1)
					continue
					//log.Fatalf("Read operation failed: %v", err)
				}
				if len(r.GetErr()) == 0 {
					fmt.Println("final value is: ", r.GetRes())
					break
				} else {
					fmt.Println("response is: ", r.GetErr())
					break
				}
			}
		} else if data.v == 4 {
			address := data.host
			//establishing the connection
			conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Fatalf("did not connect: %v", err)
			}
			defer conn.Close()
			c := pb.NewTokenClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			l, _ := strconv.ParseUint(os.Args[8], 10, 64)
			m, _ := strconv.ParseUint(os.Args[10], 10, 64)
			h, _ := strconv.ParseUint(os.Args[12], 10, 64)
			// sending write token request
			r, err := c.WriteToken(ctx, &pb.Wkey{Key: &pb.Key{Id: os.Args[4]}, Name: os.Args[6], Low: l, Mid: m, High: h})
			if err != nil {
				log.Fatalf("Write operation Failed: %v", err)
			}
			if len(r.GetErr()) == 0 {
				fmt.Println("partial value is: ", r.GetRes())
			} else {
				fmt.Println("response is: ", r.GetErr())
			}

		} else if data.v == 3 {
			address := data.host
			//establishing the connection
			//performing drop operation by connnecting to writer node
			conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				log.Fatalf("did not connect: %v", err)
			}
			defer conn.Close()
			c := pb.NewTokenClient(conn)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			_, err1 := c.DropToken(ctx, &pb.Key{Id: os.Args[3], Flag: "main"})
			if err1 != nil {
				log.Fatalf("Drop Operation failed: %v", err1)
			}
		}

	}
}
