package gateboard

type Client struct{}

type ClientOptions struct{}

func NewClient(options ClientOptions) *Client {
	return &Client{}
}
