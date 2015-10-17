# cim
Cim golang package is a generic tcp client/server that can provide arbitrary CLI services
## To install do
1. `$ go get -u github.com/LDCS/cim`
2. Create a cfg file in the format specified in example_ports.cfg based on yout needs.
3. Edit the following line in cim.go with the path to ports cfg file.
   
  `var cfgFile = "/path/to/ports.cfg"`
