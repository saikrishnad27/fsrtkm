go run client/client.go tokenclient -write -id 124 -name abcd -low 0 -mid 10000 -high 100000
go run client/client.go tokenclient -read -id 123
go run client/client.go tokenclient -read -id 124
go run client/client.go tokenclient -write -id 12345 -name abd -low 0 -mid 10 -high 100


