package global

import "os"

var DataDir = os.Getenv("DATA_DIR")
var DatabaseDir string
var ImageDir string
