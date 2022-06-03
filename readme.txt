Description: the Description of the project is given here "https://www.csee.umbc.edu/~kalpakis/courses/621-sp22/project/GoTokens-2.pdf"
solution:
 project2:
*In this project we have implement client service token manager and 
we have added the functionalities like create, read, drop, write for the tokens

*first we have to create a proto file and then we have to execute it by using scripts/proto.Sh

*after creating the proto file we have to create client server go files and implement the functionality based on the proto file in that go files

*first execute server.go file and then execute the client.go file

*the libraries that are needed to execute this file was:
    "context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"sync"
	"syscall"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
    "time"
	"gopkg.in/yaml.v3"
*Along with these libraries we need to install protoc
*I have executed this file in windows operating system and in visual studio code



for project3:

execution run the server shell scripts first and then client
scripts/server1.sh
scripts/server.sh
scripts/server2.sh

scripts/client.sh
scripts/client1.sh

considered all the cases like :
	concurrent read and write and 
	write of read concurrent with normal write
	normal read and write

In addition to this have written code for token deletion and creation also for multiple servers

shown fail silent behavior on token 124 while performing write 

