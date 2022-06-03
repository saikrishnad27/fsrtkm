go run client1/client.go tokenclient -write -id 123 -name abcd -low 0 -mid 20 -high 132
go run client1/client.go tokenclient -write -id 12345 -name abd -low 0 -mid 40 -high 60
go run client1/client.go tokenclient -read -id 124
go run client1/client.go tokenclient -read -id 12345
go run client1/client.go tokenclient -write -id 12456 -name abd -low 0 -mid 40 -high 60