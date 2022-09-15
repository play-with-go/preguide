package ci

import "github.com/SchemaStore/schemastore/src/schemas/json"

workflows: [...{file: string, schema: json.#Workflow}]
workflows: [
	{file: "test.yml", schema:       test},
	{file: "dockerSelf.yml", schema: dockerSelf},
]

#checkoutCode: {
	name: "Checkout code"
	uses: "actions/checkout@v2"
}
#installGo: {
	name: "Install Go"
	uses: "actions/setup-go@v2"
	with: "go-version": "${{ matrix.go_version }}"
}
#dockerBuildSelf: {
	name: "Generate Docker self"
	run:  "./_scripts/dockerBuildSelf.sh"
}

#installQemu: {
	name: "Install qemu"
	uses: "docker/setup-qemu-action@v1"
}
#installBuildx: {
	name: "Setup buildx"
	uses: "docker/setup-buildx-action@v1"
}

_#ubuntuLatest: "ubuntu-22.04"
_#latestGo:     "1.19.1"

test: json.#Workflow & {
	name: "Test"
	env: {
		PREGUIDE_IMAGE_OVERRIDE: "playwithgo/go1.15.15:6967d577188719cbf7a77cc80c5960695a20b103"
		PREGUIDE_PULL_IMAGE:     "missing"
	}
	on: {
		push: branches: ["main"]
		pull_request: branches: ["**"]
	}
	jobs: test: {
		strategy: {
			"fail-fast": false
			matrix: {
				os: [_#ubuntuLatest]
				go_version: [_#latestGo]
			}
		}
		"runs-on": "${{ matrix.os }}"
		steps: [
			#installQemu,
			#installBuildx,
			#checkoutCode,
			#installGo,
			{
				name: "Verify"
				run:  "go mod verify"
			},
			{
				name: "Generate"
				run:  "go generate ./..."
			},
			{
				name: "Test"
				run:  "go test ./..."
				env: CGO_ENABLED: "0"
			},
			{
				name: "Race test"
				run:  "go test -race ./..."
				if:   "${{ github.ref == 'main' }}"
				env: CGO_ENABLED: "0"
			},
			{
				name: "staticcheck"
				run:  "go run honnef.co/go/tools/cmd/staticcheck ./..."
			},
			{
				name: "Tidy"
				run:  "go mod tidy"
			},
			#dockerBuildSelf,
			{
				name: "Verify commit is clean"
				run:  #"test -z "$(git status --porcelain)" || (git status; git diff; false)"#
			},
		]
	}
}

dockerSelf: json.#Workflow & {
	name: "Docker self"
	env: {
		DOCKER_HUB_USER:  "playwithgopher"
		DOCKER_HUB_TOKEN: "${{ secrets.DOCKER_HUB_TOKEN }}"
	}
	on: {
		push: branches: ["main"]
		push: tags: ["v*"]
	}
	jobs: test: {
		strategy: {
			"fail-fast": false
			matrix: {
				os: [_#ubuntuLatest]
				go_version: [_#latestGo]
			}
		}
		"runs-on": "${{ matrix.os }}"
		steps: [
			#installQemu,
			#installBuildx,
			#checkoutCode,
			#installGo,
			#dockerBuildSelf,
		]
	}
}
