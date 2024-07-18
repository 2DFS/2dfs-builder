import os
from random import getrandbits
import subprocess
import json


REPEAT = 5
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

def gen_2dfs_manifest(files_list):
    manifest = {
        "allotments":[]
    }
    i = 0
    for f in files_list:
        manifest["allotments"].append({
            "src":f,
            "dst":"/file"+str(i),
            "row":i,
            "col":i
        })
        i += 1

    #delete 2dfs.json file if it exists
    if os.path.exists("2dfs.json"):
        os.remove("2dfs.json")

    #write manifest to 2dfs.json file
    with open("2dfs.json", "w") as f:
        json.dump(manifest, f)

def gen_dockerfile(files_list):
    dockerfile = "FROM ubuntu:22.04\n"
    i = 0
    for f in files_list:
        dockerfile += "COPY "+f+" /file"+str(i)+"\n"
        i += 1

    #delete 2dfs.json file if it exists
    if os.path.exists("Dockerfile"):
        os.remove("Dockerfile")

    #write manifest to 2dfs.json file
    with open("Dockerfile", "w") as f:
        f.write(dockerfile)

def cleanup_dir(dir):
    for file in os.listdir(dir):
        os.remove(os.path.join(dir, file))


def build_tdfs():
   cmd = ["tdfs", "build", "ubuntu:22.04","test:v1","--platforms", "linux/amd64"]
   execute_with_live_output(cmd)

def build_docker():
   cmd = ["docker", "build", "-t","test:v1","."]
   execute_with_live_output(cmd)

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
    cmd = ["tdfs", "image", "rm","-a"]
    execute_with_live_output(cmd)

def cleanup_docker():
    cmd = ["docker", "system", "prune","-a","-f"]
    execute_with_live_output(cmd)

if __name__ == "__main__":
    cleanup_tdfs()
    cleanup_docker()
    
    for e in EXPERIMENT_ALLOTMENT_RATIO:

        print("\n ##EXPERIMENT CONFIG ## \n",str(e))

        for r in range(REPEAT):

            print("\n ##EXPERIMENT RUN ",r,"## \n")
        
            
            files_list = []
            for j in range(e["allotments"]):
                filename = "files/f"+str(j)
                create_random_file(e["size"], filename)
                files_list.append(filename)

            ## TDFS EXPERIMENT
            # Generate 2dfs manifest
            print("###TDFS EXPERIMENT##")
            gen_2dfs_manifest(files_list)
            build_tdfs()
            cleanup_tdfs()

            ## DOCKER EXPERIMENT
            print("###DOCKER EXPERIMENT##")
            gen_dockerfile(files_list)
            build_docker()
            cleanup_docker()


            ## cleanup files
            cleanup_dir("./files")