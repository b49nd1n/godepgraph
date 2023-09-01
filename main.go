package main

import (
	"flag"
	"fmt"
	"go/build"
	"log"
	"os"
	"sort"
	"strings"
	"path/filepath"
)

var (
	pkgs        map[string]*build.Package
	erroredPkgs map[string]bool
	ids         map[string]string

	ignored = map[string]bool{
		"C": true,
	}
	ignoredPrefixes []string
	onlyPrefixes    []string

	ignoreStdlib   = flag.Bool("nostdlib", false, "ignore packages in the Go standard library")
	ignoreVendor   = flag.Bool("novendor", false, "ignore packages in the vendor directory")
	stopOnError    = flag.Bool("stoponerror", true, "stop on package import errors")
	withGoroot     = flag.Bool("withgoroot", false, "show dependencies of packages in the Go standard library")
	ignorePrefixes = flag.String("ignoreprefixes", "", "a comma-separated list of prefixes to ignore")
	ignorePackages = flag.String("ignorepackages", "", "a comma-separated list of packages to ignore")
	onlyPrefix     = flag.String("onlyprefixes", "", "a comma-separated list of prefixes to include")
	tagList        = flag.String("tags", "", "a comma-separated list of build tags to consider satisfied during the build")
	horizontal     = flag.Bool("horizontal", false, "lay out the dependency graph horizontally instead of vertically")
	withTests      = flag.Bool("withtests", false, "include test packages")
	maxLevel       = flag.Int("maxlevel", 256, "max level of go dependency graph")
	cwd			   = flag.String("cwd","","current work dir")
	recursive	   = flag.Bool("recursively",false, "search for .mod files recursively")
	verbose 	   = flag.Bool("verbose",false, "verbose output")
	output 	       = flag.String("output","output.dg", "output file name")

	buildTags    []string
	buildContext = build.Default
)

func init() {
	flag.BoolVar(ignoreStdlib, "s", false, "(alias for -nostdlib) ignore packages in the Go standard library")
	flag.StringVar(ignorePrefixes, "p", "", "(alias for -ignoreprefixes) a comma-separated list of prefixes to ignore")
	flag.StringVar(ignorePackages, "i", "", "(alias for -ignorepackages) a comma-separated list of packages to ignore")
	flag.StringVar(onlyPrefix, "o", "", "(alias for -onlyprefixes) a comma-separated list of prefixes to include")
	flag.BoolVar(withTests, "t", false, "(alias for -withtests) include test packages")
	flag.IntVar(maxLevel, "l", 256, "(alias for -maxlevel) maximum level of the go dependency graph")
	flag.BoolVar(withGoroot, "d", false, "(alias for -withgoroot) show dependencies of packages in the Go standard library")
	flag.StringVar(cwd, "c", "", "(alias for -cwd) set current work dir")
	flag.BoolVar(recursive, "r", false, "(alias for -recursively) search for .mod files recursively")
	flag.BoolVar(verbose, "v", false, "(alias for -verbose) verbose output")
	flag.StringVar(output, "f", "output.dg", "(alias for -output) output file")
}

func main() {
	pkgs = make(map[string]*build.Package)
	erroredPkgs = make(map[string]bool)
	ids = make(map[string]string)
	flag.Parse()

	args := flag.Args()

	if len(args) < 1 {
		log.Fatal("need one package name to process")
	}

	if *ignorePrefixes != "" {
		ignoredPrefixes = strings.Split(*ignorePrefixes, ",")
	}
	if *onlyPrefix != "" {
		onlyPrefixes = strings.Split(*onlyPrefix, ",")
	}
	if *ignorePackages != "" {
		for _, p := range strings.Split(*ignorePackages, ",") {
			ignored[p] = true
		}
	}
	if *tagList != "" {
		buildTags = strings.Split(*tagList, ",")
	}
	buildContext.BuildTags = buildTags


	default_dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get cwd: %s", err)
	}
	

	if len(*cwd) == 0 {
		*cwd, err = os.Getwd()
		if err != nil {
			log.Fatalf("failed to get cwd: %s", err)
		}
	}else{
		os.Chdir(*cwd)
	}

	var root_paths = make(map[string]string)

	if *recursive {

	    _ = filepath.Walk(*cwd, func(path string, file os.FileInfo, err error) error {

	        if err != nil {
	            fmt.Println(err)
	            return nil
	        }
	        dir:=filepath.Dir(path)
	        for p,_ := range root_paths{
	        	if strings.Contains(dir, p) {
//	        		log.Printf("Already has %s, skipping %s",p, path)
	        		return nil
	        	}
	        }

	        if !file.IsDir() && filepath.Base(path) == "go.mod"{
	        	root_paths[filepath.Dir(path)] = filepath.Join(*cwd,path)
				debugf("Found mod.file in path %s", path)
	        }

	        return nil
    	})
	}else{
		root_paths[*cwd]=filepath.Join(*cwd,"go.mod")
	}

	for mod_path, _ := range root_paths {
		debugf("Entering %s",mod_path)
		os.Chdir(mod_path)
		for _, a := range args {
			if err := processPackage(mod_path, a, 0, "", *stopOnError); err != nil {
				log.Fatal(err)
			}
		}
		debugf("Leaving %s",mod_path)		
	}

    f, err := os.Create(filepath.Join(default_dir,*output))
    defer f.Close()

	// fmt.Println("digraph godep {")
	f.WriteString("digraph godep {")
	if *horizontal {
		// fmt.Println(`rankdir="LR"`)
		f.WriteString(`rankdir="LR"`)
	}
	f.WriteString(`splines=ortho
nodesep=0.4
ranksep=0.8
node [shape="box",style="rounded,filled"]
edge [arrowsize="0.5"]
`)

	// sort packages
	pkgKeys := []string{}
	for k := range pkgs {
		pkgKeys = append(pkgKeys, k)
	}
	sort.Strings(pkgKeys)
	// fmt.Println("%v",pkgKeys)

	for _, pkgName := range pkgKeys {
		pkg := pkgs[pkgName]
		pkgId := getId(pkgName)

		if isIgnored(pkg) {
			continue
		}


		var color string
		switch {
		case strings.Contains(pkgName,"solidwall.io"):
			color = "yellow"
		case pkg.Goroot:
			color = "palegreen"
		case len(pkg.CgoFiles) > 0:
			color = "darkgoldenrod"
		case isVendored(pkg.ImportPath):
			color = "palegoldenrod"
		case hasBuildErrors(pkg):
			color = "red"
		default:
			color = "paleturquoise"
		}


		f.WriteString(fmt.Sprintf("%s [label=\"%s\" color=\"%s\" URL=\"%s\" target=\"_blank\"];\n", pkgId, pkgName, color, pkgDocsURL(pkgName)))


		// Don't render imports from packages in Goroot
		if pkg.Goroot && !*withGoroot {
			continue
		}

		for _, imp := range getImports(pkg) {
			impPkg := pkgs[imp]
			if impPkg == nil || isIgnored(impPkg) {
				continue
			}

			impId := getId(imp)
			f.WriteString(fmt.Sprintf("%s -> %s;\n", pkgId, impId))
		}
	}
	f.WriteString("}")
}

func pkgDocsURL(pkgName string) string {
	return pkgName
}

func processPackage(root string, pkgName string, level int, importedBy string, stopOnError bool) error {
	if level++; level > *maxLevel {
		return nil
	}
	if ignored[pkgName] {
		return nil
	}

	debugf("Importing %s in %s\n", pkgName, root)
	pkg, buildErr := buildContext.Import(pkgName, root, 0)
	if buildErr != nil {
		if stopOnError {
			return fmt.Errorf("failed to import %s (imported at level %d by %s):\n%s", pkgName, level, importedBy, buildErr)
		}
	}

	if isIgnored(pkg) {
		return nil
	}

	importPath := normalizeVendor(pkgName)
	if buildErr != nil {
		erroredPkgs[importPath] = true
	}

	pkgs[importPath] = pkg

	// Don't worry about dependencies for stdlib packages
	if pkg.Goroot && !*withGoroot {
		return nil
	}

	for _, imp := range getImports(pkg) {
		if _, ok := pkgs[imp]; !ok {
			if err := processPackage(pkg.Dir, imp, level, pkgName, stopOnError); err != nil {
				return err
			}
		}
	}
	return nil
}

func getImports(pkg *build.Package) []string {
	allImports := pkg.Imports
	if *withTests {
		allImports = append(allImports, pkg.TestImports...)
		allImports = append(allImports, pkg.XTestImports...)
	}
	var imports []string
	found := make(map[string]struct{})
	for _, imp := range allImports {
		if imp == normalizeVendor(pkg.ImportPath) {
			// Don't draw a self-reference when foo_test depends on foo.
			continue
		}
		if _, ok := found[imp]; ok {
			continue
		}
		found[imp] = struct{}{}
		imports = append(imports, imp)
	}
	return imports
}

func deriveNodeID(packageName string) string {
	//TODO: improve implementation?
	id := "\"" + packageName + "\""
	return id
}

func getId(name string) string {
	id, ok := ids[name]
	if !ok {
		id = deriveNodeID(name)
		ids[name] = id
	}
	return id
}

func hasPrefixes(s string, prefixes []string) bool {
	for _, p := range prefixes {
		// debugf("%s has prefix %s ?",s,p)
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func isIgnored(pkg *build.Package) bool {
	if len(onlyPrefixes) > 0 && !hasPrefixes(normalizeVendor(pkg.ImportPath), onlyPrefixes) {
		return true
	}

	if *ignoreVendor && isVendored(pkg.ImportPath) {
		return true
	}
	return ignored[normalizeVendor(pkg.ImportPath)] || (pkg.Goroot && *ignoreStdlib) || hasPrefixes(normalizeVendor(pkg.ImportPath), ignoredPrefixes)
}

func hasBuildErrors(pkg *build.Package) bool {
	if len(erroredPkgs) == 0 {
		return false
	}

	v, ok := erroredPkgs[normalizeVendor(pkg.ImportPath)]
	if !ok {
		return false
	}
	return v
}

func debug(args ...interface{}) {
	fmt.Fprintln(os.Stderr, args...)
}

func debugf(s string, args ...interface{}) {
	if(*verbose){
		fmt.Println(fmt.Sprintf(s, args...))
	}

}

func isVendored(path string) bool {
	return strings.Contains(path, "/vendor/")
}

func normalizeVendor(path string) string {
	pieces := strings.Split(path, "vendor/")
	return pieces[len(pieces)-1]
}
