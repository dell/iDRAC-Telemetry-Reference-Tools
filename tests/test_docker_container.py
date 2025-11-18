import pytest
import time
from commonhelper import *

container_list = [
    '--influx-test-db',
    '--splunk-pump',
    '--elk-pump',
    '--timescale-pump',
    '--victoria-pump --victoria-db',
    '--prometheus-test-db',
    '--otel-pump',
    '--kafka-pump',
]
setup_container_list = ['--prometheus-test-db','--influx-test-db']

class TestDockerContainer:
    """
        Checks the status of a Docker container.

        Args:
            container_name (str): The name of the Docker container.

        Returns:
            None

        Raises:
            AssertionError: If the container is not up.
    """
    def is_docker_container_up(self, container_name):
        container_name_short = self.get_container_short_name(container_name)
        print(f"Checking status for: {container_name_short}")
        out = local_exec(f"docker ps -a | grep '{container_name_short}'")
        assert 'Up' in out, f"{container_name} container is not up"
        print(f"{container_name} container is up")

    def get_container_short_name(self, container_name):
        return container_name.split()[0].lstrip('-').split('-')[0]

    """
        This function tests the setup of a container by calling a setup command from a compose.sh file.
        The function takes a container name as a parameter, which is a value from the setup_container_list list.
        The function returns None.
    """
    @pytest.mark.parametrize("container", setup_container_list)
    def test_build_setup(self, container):
        print(f"Setting up container for option: {container}")
        cmd1 = "../docker-compose-files/compose.sh setup " + container
        out = local_exec(cmd1)
        time.sleep(10)
        container_name_short = self.get_container_short_name(container)
        assert container_name_short.capitalize() + " pump container setup done." in out, f"{container} setup failed: {out}"
        print(f"{container} setup done successfully" )

    """
        Tests the start and stop functionality of a container.
        Parameters:
            container (str): The name of the container to be tested.
        Return Type:
            None
    """
    @pytest.mark.parametrize("container", container_list)
    def test_docker_container_start_stop(self, container):
        print(f"Starting container: {container}")
        out = local_exec("../docker-compose-files/compose.sh start " + container)
        time.sleep(5)
        assert "Created" in out or "Started" in out, f"{container} not started successfully: {out}"
        print(f"{container} started successfully")

        self.is_docker_container_up(container)

        out = local_exec("../docker-compose-files/compose.sh stop " + container)
        assert "Stopped" in out, f"{container} did not stop successfully: {out}"
        print(f"{container} stopped successfully")

    """
    This function tests the cleanup of a container by stopping it if it's running, removing it, 
    and optionally pruning unused containers and volumes. It takes a container name as a 
    parameter and returns None.
    """
    @pytest.mark.parametrize("container", container_list)
    def test_run_cleanup(self, container):
        
        print(f"Cleaning up container: {container}")
        container_name_short = self.get_container_short_name(container)
        # Stop container if running
        local_exec(f"docker stop $(docker ps -q --filter name={container_name_short}) || true")

        # Remove container
        local_exec(f"docker rm $(docker ps -a -q --filter name={container_name_short}) || true")

        # Optional: prune unused containers and volumes
        local_exec("docker container prune -f")
        local_exec("docker volume prune -f")

        # Validate cleanup for this container
        out = local_exec("docker ps -a --format '{{.Names}}'")
        assert container_name_short not in out, f"Cleanup failed for {container_name_short}"
        print(f"{container_name_short} cleaned successfully")

    """
        This function tests the cleanup of a Docker network. It takes no parameters and returns None.
    """
    def test_cleanup_network():
        local_exec("docker network rm idrac-telemetry-reference-tools_host-bridge-net || true")
        networks = local_exec("docker network ls --format '{{.Name}}'")
        assert "idrac-telemetry-reference-tools_host-bridge-net" not in networks
        print("Network cleaned successfully")