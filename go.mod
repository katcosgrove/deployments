module github.com/mendersoftware/deployments/v2

go 1.13

require (
	github.com/ant0ine/go-json-rest v3.3.3-0.20170913041208-ebb33769ae01+incompatible
	github.com/asaskevich/govalidator v0.0.0-20190424111038-f61b66f89f4a
	github.com/aws/aws-sdk-go v1.25.14
	github.com/globalsign/mgo v0.0.0-20181015135952-eeefdecb41b8
	github.com/gofrs/uuid v3.2.0+incompatible
	github.com/mendersoftware/go-lib-micro v0.0.0-20190308213250-cb210e0b60d9
	github.com/mendersoftware/mender-artifact v0.0.0-20191014104918-d5f36327b36f
	github.com/mendersoftware/mendertesting v0.0.0-20180410095158-9e728b524c29
	github.com/pkg/errors v0.8.1
	github.com/remyoudompheng/go-liblzma v0.0.0-20190506200333-81bf2d431b96 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/sirupsen/logrus v1.4.2
	github.com/spf13/viper v1.4.0 // indirect
	github.com/stretchr/testify v1.4.0
	github.com/urfave/cli v1.22.1
	golang.org/x/net v0.0.0-20191014212845-da9a3fd4c582 // indirect
)

replace github.com/mendersoftware/go-lib-micro => ../go-lib-micro
