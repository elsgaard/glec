go run main.go node.go config.go -id=localhost:8000 -peers=localhost:8001,localhost:8002
go run main.go node.go config.go -id=localhost:8001 -peers=localhost:8000,localhost:8002
go run main.go node.go config.go -id=localhost:8002 -peers=localhost:8000,localhost:8001