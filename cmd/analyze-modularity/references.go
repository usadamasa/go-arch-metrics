package main

// exportedUsage はパッケージの exported のうち、外から参照されないものの名前｡
type exportedUsage struct {
	Unreferenced []string
	TestOnly     []string
}

// analyzeExportedReferences は listed の main でないパッケージごとに、外から参照されない exported を返す｡
func analyzeExportedReferences(moduleRoot string, listed []listedPackage) (map[string]exportedUsage, error) {
	return nil, nil
}
