package config

import (
	"fmt"
	"net/url"
)

func urlEscape(s string) string { return url.QueryEscape(s) }

// URL returns an amqp:// URL with credentials escaped.
func (r RabbitMQ) URL() string {
	vhost := r.VHost
	if vhost == "/" {
		vhost = "" // amqp091 treats an empty path as the default vhost
	}
	return fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		urlEscape(r.User), urlEscape(r.Password), r.Host, r.Port, urlEscape(vhost))
}
