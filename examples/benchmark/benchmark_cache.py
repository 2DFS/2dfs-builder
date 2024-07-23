import os
from random import getrandbits
import subprocess
import json
from datetime import datetime
import csv
import time
from utils.utils import *


REPEAT = 5
COOLDOWN = 10 #SECONDS
EXPERIMENT_ALLOTMENT_RATIO = [
    {
        "size": 500,
        "allotments": 2,
        "change": 1
    },
    {
        "size": 10,
        "allotments": 100,
        "change": 1
    },
    {
        "size": 10,
        "allotments": 100,
        "change": 1
    },
    {
        "size": 5,
        "allotments": 200,
        "change": 1
    },
    {
        "size": 10,
        "allotments": 100,
        "change": 1
    },
    {
        "size": 10,
        "allotments": 100,
        "change": 10
    },
    {
        "size": 10,
        "allotments": 100,
        "change": 50
    }
]

if __name__ == "__main__":
    csvoutput = [
        ["tool","allotments","size","changed","tot","download","layering"]
    ]
    cleanup_tdfs()
    cleanup_docker()
    
    for e in EXPERIMENT_ALLOTMENT_RATIO:

        print("\n ##EXPERIMENT CONFIG ## \n",str(e))

        for r in range(REPEAT):

            print("\n ##COOLDOWN## \n")
            time.sleep(COOLDOWN)

            print("\n ##EXPERIMENT RUN ",r,"## \n")
        
            
            files_list = []
            for j in range(e["allotments"]):
                filename = "files/f"+str(j)
                create_random_file(e["size"], filename)
                files_list.append(filename)

            ## TDFS EXPERIMENT
            # Generate 2dfs manifest
            print("###TDFS EXPERIMENT##")
            print("Cold build...")
            gen_2dfs_manifest(files_list)
            result = build_tdfs()
            total, download_time, layering_time = parse_tdfs_output(result)
            print("Total time: ",total, "Download time", download_time, "Layering time", layering_time)
            print("Change allotments...")
            for j in range(e["allotments"]):
                filename = "files/f"+str(j)
                os.remove(filename)
                create_random_file(e["size"], filename)
            print("Warm build...")
            result = build_tdfs()
            total, download_time, layering_time = parse_tdfs_output(result)
            print("Total time: ",total, "Download time", download_time, "Layering time", layering_time)
            csvoutput.append(["tdfs",e["allotments"],e["size"],e["change"],total,download_time,layering_time])
            cleanup_tdfs()

            ## DOCKER EXPERIMENT
            print("###DOCKER EXPERIMENT##")
            print("Cold build...")
            gen_dockerfile(files_list)
            result = build_docker()
            total, download_time, layering_time = parse_docker_output(result)
            print("Total time: ",total, "Download time", download_time, "Layering time", layering_time)
            print("Change allotments...")
            for j in range(e["allotments"]):
                filename = "files/f"+str(j)
                os.remove(filename)
                create_random_file(e["size"], filename)
            print("Warm build...")
            result = build_docker()
            total, download_time, layering_time = parse_docker_output(result)
            print("Total time: ",total, "Download time", download_time, "Layering time", layering_time)
            csvoutput.append(["docker",e["allotments"],e["size"],e["change"],total,download_time,layering_time])
            cleanup_docker()


            ## cleanup files
            cleanup_dir("./files")

            ##exporting results to csv
            csvname = "results.csv"
            try:     
                os.remove(csvname)
            except:
                pass
            with open(csvname, "w", newline="") as csvfile:
                writer = csv.writer(csvfile)
                writer.writerows(csvoutput)