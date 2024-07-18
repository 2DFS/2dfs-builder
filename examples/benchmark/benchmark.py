import os
from random import getrandbits
import subprocess


EXPERIMENT_ALLOTMENT_RATIO = [
    {
        "size": 100,
        "allotments": 10
    },
    {
        "size": 1000,
        "allotments": 1
    },
    {
        "size": 500,
        "allotments": 5
    }
]

def create_random_file(file_size_mb, filename):
    """
    Creates a file of specified size (in MB) filled with random bytes.

    Args:
        file_size_mb: The desired size of the file in megabytes.
        filename: The name of the file to create.
    """
    file_size_bytes = file_size_mb * 1024 * 1024

    with open(filename, "wb") as f:
        while file_size_bytes > 0:
            # Generate random bytes (chunk size of 1 MB)
            random_bytes = getrandbits(8 * 1024 * 1024)  # 1 MB
            # Write the random bytes to the file
            f.write(random_bytes.to_bytes(1024 * 1024, byteorder='big'))
            # Update remaining bytes
            file_size_bytes -= 1024 * 1024

def cleanup_dir(dir):
    for file in os.listdir(dir):
        os.remove(os.path.join(dir, file))


def build_tdfs():
   pass

def exec_command(command):
    try:
        result = subprocess.run(command, check=True, capture_output=True)
        # Decode the output from bytes to string
        output = result.stdout.decode("utf-8")
        print(output)
    except subprocess.CalledProcessError as error:
        print(f"Error executing command: {error}")


def execute_with_live_output(cmd):
    """
    Executes a command and prints its output line by line as it becomes available.

    Args:
        cmd: The command to execute as a list of arguments.
    """
    # Open a pipe for stdout
    process = subprocess.Popen(cmd, stdout=subprocess.PIPE, universal_newlines=True)

    # Iterate over the output lines
    for line in iter(process.stdout.readline, ""):
        print(line, end="")  # Print without newline

    # Wait for the process to finish and close the pipe
    process.stdout.close()
    process.wait()

def cleanup_tdfs():
    pass

if __name__ == "__main__":
    execute_with_live_output(["ls","-al"])