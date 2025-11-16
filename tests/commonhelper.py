import subprocess
import os

def get_cwd():
    """Get the current working directory."""
    return os.getcwd()

def local_exec(cmd):
    """Execute a command locally and return output."""
    print(f"Executing: {cmd}")
    pipe = subprocess.Popen(cmd, shell=True, cwd=get_cwd(),
                            stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE)
    stdout, stderr = pipe.communicate()
    if not stderr or len(stderr.strip()) == 0:
        out = stdout.decode()
    else:
        out = stdout.decode() + '\n' + stderr.decode()
    return out