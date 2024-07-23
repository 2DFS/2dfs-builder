import os
from random import getrandbits
import subprocess
import json
from datetime import datetime
import csv
import time


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
   cmd = ["time","tdfs", "build", "ubuntu:22.04","test:v1","--platforms", "linux/amd64"]
   return exec_command(cmd)

def build_docker():
   cmd = ["time","docker", "build", "-t","test:v1","."]
   return exec_command(cmd)

def exec_command(command):
    try:
        result = subprocess.run(command, check=True, capture_output=True)
        #decode to utf8
        output = (result.stdout + result.stderr).decode("utf-8")
        return output
    except subprocess.CalledProcessError as error:
        print(f"Error executing command: {error}")
    return ""

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
    exec_command(cmd)

def cleanup_docker():
    cmd = ["docker", "system", "prune","-a","-f"]
    exec_command(cmd)

def parse_tdfs_output(output):
    total = 0
    begin_time = 0
    download_time = 0
    layering_time = 0
    for line in output.split("\n"):
        linearr = line.split(" ")
        for i,token in enumerate(linearr):
            # extract total time
            if "elapsed" in token:
                #remove elapsed from token
                token = token.replace("elapsed","")
                total = parse_time_output(token)
            # extract experiment begin time
            if "Parsing" in token:
                begin_time = parse_time_to_millis(linearr[i-1])
            # extract download finished time
            if "retrieved" in token:
                download_time = parse_time_to_millis(linearr[i-3])-begin_time
            # extract copy time time
            if "[COPY]" in token:
                layering_time = (parse_time_to_millis(linearr[i-3])-begin_time)-download_time
    return total, download_time, layering_time

def parse_docker_output(output):
    total = 0
    begin_time = 0
    download_time = 0
    layering_time = 0
    tempsum = 0.0 
    for line in output.split("\n"):
        linearr = line.split(" ")
        if "#5" in line:
            download_time += tempsum
            tempsum = 0.0
        if "exporting to image" in line:
            layering_time += tempsum
            tempsum = 0.0
            continue
        for i,token in enumerate(linearr):
            # extract total time
            if "elapsed" in token:
                #remove elapsed from token
                token = token.replace("elapsed","")
                total = parse_time_output(token)
            # extract experiment begin time
            if "DONE" in token:
                tempsum+=float(linearr[i+1].replace("s",""))
    return total, download_time, layering_time

def parse_time_to_millis(time_str):
  try:
    time_obj = datetime.strptime(time_str, "%H:%M:%S")
  except ValueError:
    raise ValueError(f"Invalid time format: {time_str}")

  return time_obj.timestamp()

def parse_time_output(time_str):
    timeplit = time_str.split(":")
    minutes = int(timeplit[0])
    seconds = float(timeplit[1])
    return float(minutes*60)+seconds
