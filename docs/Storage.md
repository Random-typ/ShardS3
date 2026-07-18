# Storage
Each S3-Object is split into **chunks** which are then split into **shards**, the shards are then encoded with **reed-solomon**  
encoding and stored accross multiple **backends**. The Objects are split in a way that guarantees a variable amount of backend failures  
before data is lost. 

## Chunks

## Shards

## Backends 

## Metadata
Metadata is stored in a simple SQLite Database (./filepath). The metadata contains information  
about each S3-Object as well as information about each **chunk** and **shard**

## Chunking and Sharding
