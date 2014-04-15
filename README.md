grange-server
=============

A proof-of-concept server making use of https://github.com/xaviershay/grange

Put some range YAML files in `clusters/`, then:

    go run logging.go server.go -port 8888

    gem install rangeclient
    er -v localhost -p 8888 '%your-cluster'
