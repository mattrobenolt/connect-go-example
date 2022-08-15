package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/bufbuild/connect-go"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	greetv1 "example/gen/greet/v1"
	"example/gen/greet/v1/greetv1connect"
)

const Name = "proto"

type codec struct{}

type vtprotoMessage interface {
	MarshalVT() ([]byte, error)
	UnmarshalVT([]byte) error
}

func (*codec) Marshal(v any) ([]byte, error) {
	switch vv := v.(type) {
	case vtprotoMessage:
		return vv.MarshalVT()
	case proto.Message:
		return proto.Marshal(vv)
	}
	return nil, fmt.Errorf("failed to marshal, message is %T, want proto.Message", v)
}

func (*codec) Unmarshal(data []byte, v any) error {
	switch vv := v.(type) {
	case vtprotoMessage:
		return vv.UnmarshalVT(data)
	case proto.Message:
		return proto.Unmarshal(data, vv)
	}
	return fmt.Errorf("failed to unmarshal, message is %T, want proto.Message", v)
}

func (*codec) Name() string {
	return "proto"
}

func (*codec) String() string {
	return "proto"
}

type GreetServer struct{}

func (s *GreetServer) Greet(
	ctx context.Context,
	req *connect.Request[greetv1.GreetRequest],
	resp *connect.ServerStream[greetv1.GreetResponse],
) error {
	resp.Send(&greetv1.GreetResponse{
		Message: []string{"greeting 1"},
	})
	resp.Send(&greetv1.GreetResponse{
		Message: []string{"greeting 2"},
	})
	return nil
}

func main() {
	greeter := &GreetServer{}
	mux := http.NewServeMux()
	mux.Handle(greetv1connect.NewGreetServiceHandler(greeter))

	srv := &http.Server{
		Addr:    "127.0.0.1:8080",
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		srv.ListenAndServe()
	}()

	time.Sleep(50 * time.Millisecond)

	connectClient := greetv1connect.NewGreetServiceClient(
		http.DefaultClient,
		"http://127.0.0.1:8080",
		connect.WithCodec(&codec{}),
	)
	connectStream, err := connectClient.Greet(context.Background(), connect.NewRequest(&greetv1.GreetRequest{
		Name: "matt",
	}))
	if err != nil {
		panic(err)
	}

	for connectStream.Receive() {
		msg := connectStream.Msg()
		fmt.Printf("connect: Got greeting %p: %#v\n", msg, msg.Message)
		// msg.Reset()
	}

	if err := connectStream.Err(); err != nil {
		panic(err)
	}

	conn, err := grpc.Dial("127.0.0.1:8080", grpc.WithInsecure(), grpc.WithCodec(&codec{}))
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	client := greetv1.NewGreetServiceClient(conn)
	grpcStream, err := client.Greet(context.Background(), &greetv1.GreetRequest{Name: "matt"})
	if err != nil {
		panic(err)
	}
	for {
		msg, err := grpcStream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		fmt.Printf("grpc-go: Got greeting %p: %#v\n", msg, msg.Message)
	}
	srv.Close()
	wg.Wait()
}
