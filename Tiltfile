# Parse config file
config.define_bool("porch-server-debug")
config.define_bool("porch-controllers-debug")
config.define_bool("porch-function-runner-debug")
cfg = config.parse()

# There are cross-dependencies, so whenever a single file is changed, everything would be redeployed with TRIGGER_MODE_AUTO
trigger_mode(TRIGGER_MODE_MANUAL)

#TODO(nagygergo): Discover these automatically?
crds = [
    "api/porchconfig/v1alpha1/config.porch.kpt.dev_functions.yaml",
    "api/porchconfig/v1alpha1/config.porch.kpt.dev_repositories.yaml",
    "controllers/config/crd/bases/config.porch.kpt.dev_fleetmembershipbindings.yaml",
    "controllers/config/crd/bases/config.porch.kpt.dev_fleetmemberships.yaml",
    "controllers/config/crd/bases/config.porch.kpt.dev_fleetscopes.yaml",
    "controllers/config/crd/bases/config.porch.kpt.dev_fleetsyncs.yaml",
    "controllers/config/crd/bases/config.porch.kpt.dev_packagevariants.yaml",
    "controllers/config/crd/bases/config.porch.kpt.dev_packagevariantsets.yaml"
]

def build_porch_server_image():
    #TODO(nagygergo): Fix build pathing hacks in dockerfile & makefile
    dockerfile = ""
    if cfg.get("porch-server-debug", False):
        dockerfile = "build/Dockerfile.debug"
    else:
        dockerfile = "build/Dockerfile"
    docker_build("nephio-project/porch-server", os.path.abspath("./.."), 
            dockerfile=dockerfile,
            build_args={"debug_port": "4000"})

def build_porch_controllers_image():
    dockerfile = ""
    if cfg.get("porch-controllers-debug", False):
        dockerfile = "controllers/Dockerfile.debug"
    else:
        dockerfile = "controllers/Dockerfile"
    docker_build("nephio-project/porch-controllers", os.path.abspath("./.."), 
            dockerfile=dockerfile,
            build_args={"debug_port": "4001"})

def build_function_runner_image():
    dockerfile = ""
    if cfg.get("porch-function-runner-debug", False):
        dockerfile = "func/Dockerfile.debug"
    else:
        dockerfile = "func/Dockerfile"
    docker_build("nephio-project/porch-function-runner", os.path.abspath("./.."), 
            dockerfile=dockerfile,
            build_args={"debug_port": "4002"})

def build_wrapper_server_image():
    dockerfile = "func/Dockerfile-wrapperserver"
    docker_build("nephio-project/porch-wrapper-server", os.path.abspath("./.."), 
            dockerfile=dockerfile, match_in_env_vars=True)


def deploy_porch_kpt_package():
    for crd in crds:
        k8s_yaml(crd)
    kptfile, rest = filter_yaml(local("kpt fn render deployments/porch -o unwrap", quiet=True), kind="Kptfile")
    k8s_yaml(rest)
    # Set up port-forwards as needed
    if cfg.get("porch-server-debug", False):
        k8s_resource(workload="porch-server", port_forwards="4000:4000")

    if cfg.get("porch-controllers-debug", False):
        k8s_resource(workload="porch-controllers", port_forwards="4001:4001")
    
    if cfg.get("porch-function-runner-debug", False):
        k8s_resource(workload="function-runner", port_forwards="4002:4002")


build_porch_server_image()
build_porch_controllers_image()
build_function_runner_image()
build_wrapper_server_image()
deploy_porch_kpt_package()