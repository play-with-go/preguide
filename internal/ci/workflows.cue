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

test: json.#Workflow & {
	name: "Test"
	env: {
		PREGUIDE_IMAGE_OVERRIDE: "playwithgo/go1.15.1@sha256:009b09b0b12874cb1dd362bc2471e39d430f6c2c7cac47dc9db2ab7a4290e59b"
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
				os: ["ubuntu-latest"]
				go_version: ["1.15.2"]
			}
		}
		"runs-on": "${{ matrix.os }}"
		steps: [
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
				os: ["ubuntu-latest"]
				go_version: ["1.15.2"]
			}
		}
		"runs-on": "${{ matrix.os }}"
		steps: [
			#checkoutCode,
			#installGo,
			#dockerBuildSelf,
		]
	}
}
