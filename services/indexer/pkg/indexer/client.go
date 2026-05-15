package indexer

import (
	"context"

	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"google.golang.org/grpc"
)

type Client struct {
	conn *grpc.ClientConn
	api  indexerv1.IndexerServiceClient
}

func Dial(ctx context.Context, address string, opts ...grpc.DialOption) (*Client, error) {
	conn, err := servicekit.Dial(ctx, address, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, api: indexerv1.NewIndexerServiceClient(conn)}, nil
}

func (c *Client) Close() error {
	return c.conn.Close()
}

func (c *Client) IndexDocument(ctx context.Context, req *indexerv1.IndexRequest, opts ...grpc.CallOption) (*indexerv1.IndexStatus, error) {
	return c.api.IndexDocument(ctx, req, opts...)
}

func (c *Client) GetContext(ctx context.Context, req *indexerv1.QueryRequest, opts ...grpc.CallOption) (*indexerv1.ContextResponse, error) {
	return c.api.GetContext(ctx, req, opts...)
}

func (c *Client) DeleteChunk(ctx context.Context, req *indexerv1.DeleteChunkRequest, opts ...grpc.CallOption) (*indexerv1.DeleteChunkResponse, error) {
	return c.api.DeleteChunk(ctx, req, opts...)
}
