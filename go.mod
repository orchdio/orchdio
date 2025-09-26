// +build !go1.20
module orchdio

go 1.24.0

toolchain go1.24.2

require (
	github.com/TheZeroSlave/zapsentry v1.23.0
	github.com/antoniodipinto/ikisocket v0.0.0-20240218211834-b7f01d1e5ec6
	github.com/badoux/goscraper v0.0.0-20190827161153-36995ce6b19f
	github.com/davecgh/go-spew v1.1.1
	github.com/getsentry/sentry-go v0.31.1
	github.com/go-redis/redis/v8 v8.11.5
	github.com/gofiber/fiber/v2 v2.52.6
	github.com/gofiber/jwt/v3 v3.3.10
	github.com/golang-jwt/jwt/v4 v4.5.2
	github.com/golang-migrate/migrate/v4 v4.18.2
	github.com/google/uuid v1.6.0
	github.com/hibiken/asynq v0.25.1
	github.com/jmoiron/sqlx v1.4.0
	github.com/joho/godotenv v1.5.1
	github.com/lib/pq v1.10.9
	github.com/minchao/go-apple-music v0.0.0-20230815040201-3b2aec2d7ffe
	github.com/nleeper/goment v1.4.4
	github.com/raitonoberu/ytmusic v0.0.0-20240324143733-0e5780514b1d
	github.com/robfig/cron/v3 v3.0.1
	github.com/samber/lo v1.49.1
	github.com/savsgio/gotils v0.0.0-20240704082632-aef3928b8a38
	github.com/sendinblue/APIv3-go-library/v2 v2.1.2
	github.com/stretchr/testify v1.10.0
	github.com/svix/svix-webhooks v1.44.0
	github.com/teris-io/shortid v0.0.0-20220617161101-71ec9f2aa569
	github.com/vicanso/go-axios v1.6.1
	github.com/zmb3/spotify/v2 v2.2.0
	go.uber.org/zap v1.27.0
	golang.org/x/crypto v0.37.0
	golang.org/x/oauth2 v0.28.0
	golang.org/x/text v0.24.0
)

require (
	dario.cat/mergo v1.0.2 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20230124172434-306776ec8161 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/Microsoft/hcsshim v0.11.4 // indirect
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/antihax/optional v1.0.0 // indirect
	github.com/cenkalti/backoff/v4 v4.2.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/containerd v1.7.11 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/log v0.1.0 // indirect
	github.com/containerd/platforms v0.2.1 // indirect
	github.com/cpuguy83/dockercfg v0.3.2 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/docker v28.3.3+incompatible // indirect
	github.com/docker/go-connections v0.6.0 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/ebitengine/purego v0.8.4 // indirect
	github.com/fasthttp/websocket v1.5.12 // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/gofiber/contrib/websocket v1.2.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/magiconair/properties v1.8.10 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/mdelapenya/tlscert v0.2.0 // indirect
	github.com/moby/docker-image-spec v1.3.1 // indirect
	github.com/moby/go-archive v0.1.0 // indirect
	github.com/moby/patternmatcher v0.6.0 // indirect
	github.com/moby/sys/sequential v0.6.0 // indirect
	github.com/moby/sys/user v0.4.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/moby/term v0.5.0 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/philhofer/fwd v1.1.3-0.20240916144458-20a13a1f6b7c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/redis/go-redis/v9 v9.7.3 // indirect
	github.com/rivo/uniseg v0.4.4 // indirect
	github.com/shirou/gopsutil/v3 v3.23.11 // indirect
	github.com/shirou/gopsutil/v4 v4.25.6 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/testcontainers/testcontainers-go v0.39.0 // indirect
	github.com/testcontainers/testcontainers-go/modules/postgres v0.39.0 // indirect
	github.com/testcontainers/testcontainers-go/modules/redis v0.39.0 // indirect
	github.com/tinylib/msgp v1.2.5 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/tkuchiki/go-timezone v0.2.3 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasthttp v1.60.0 // indirect
	github.com/vicanso/http-trace v1.2.0 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.54.0 // indirect
	go.opentelemetry.io/otel v1.35.0 // indirect
	go.opentelemetry.io/otel/metric v1.35.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/exp v0.0.0-20230510235704-dd950f8aeaea // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.36.0 // indirect
	golang.org/x/time v0.11.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240903143218-8af14fe29dc1 // indirect
	google.golang.org/grpc v1.67.0 // indirect
	google.golang.org/protobuf v1.36.6 // indirect
	gopkg.in/validator.v2 v2.0.1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
