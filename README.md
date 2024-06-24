# 2dfs-builder
Builder for 2DFS Images

## Build and Install `tdfs` cli

Install the binary inside `~/bin/tdfs` using:

```
./install.sh
```

## Get Started

Navigate to the `examples/simple-2dfs` example directory.

Use `tdfs build ubuntu:22.04 mytdfs:v1` to create your first tdfs image.

## tdfs --help

```
Requires a 2dfs.yaml file in the current directory or a path to a 2dfs.yaml file. Read docs at https://github.com/giobart/2dfs-builder

Usage:
  tdfs [command]

Available Commands:
  build       Build a 2dfs field from an oci image link
  help        Help about any command
  image       Commands to manage images
  version     Print the version number of tdfs

Flags:
  -h, --help   help for tdfs
```

## tdfs image export 


```
Usage:
  tdfs image export [reference] [targetFile] [flags]

Flags:
      --as string   export format, supported formats: tar
  -h, --help        help for export
```

You can use this to export a tdfs image a oci image using semantic labeling. 

E.g., 

```
tdfs export mytdfs:v1+0,0,1,1 image.tar.gz
```
This will export allotments (0,0),(0,1),(1,0),(1,1) as OCI layers. 

```
tdfs export mytdfs:v1+0,0,0,0 image.tar.gz
```
This will export only allotment (0,0) as OCI layer. 

## Semantic label syntax 

Given the following 3x3 field
```
   0  1  2
    __ __ __
0  |__|__|__|
1  |__|__|__|
2  |__|__|__|

```

The semantic label `image:latest+x1,y1,x2,y2` will generate a partition such that:

**allotment (a) in Field (F) iff := a.row>=x1 & a.row <= x2 & a.col>=y1 & a.col<=y2** 

Smentic labels can be chained, e.g., `image:latest+x1,y1,x2,y2+x11,y11,x22,y22+...`
and the result will be the union of all partitions. 


