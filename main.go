package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/kussell-lab/mcorr"

	"gopkg.in/alecthomas/kingpin.v2"
)

func main() {
	app := kingpin.New("mcorr-vcf", "Calculate mutational correlation from VCF files.")
	app.Version("v20171020")
	vcfFileArg := app.Arg("vcf-file", "VCF input file.").Required().String()
	outFileArg := app.Arg("out-prefix", "output prefix.").Required().String()
	maxlFlag := app.Flag("max-corr-length", "max length of correlations (bp).").Default("300").Int()
	regionStartFlag := app.Flag("region-start", "region start").Default("1").Int()
	regionEndFlag := app.Flag("region-end", "region end").Default("1000000000000").Int()
	subPopFile := app.Flag("sub-pop", "sub population list").Default("").String()
	chrom := app.Flag("chrom", "chromosome").Default("").String()
	kingpin.MustParse(app.Parse(os.Args[1:]))

	var subPopIndexMap map[int]bool
	if *subPopFile != "" {
		lines := readTrimLines(*subPopFile)
		subPopIndexMap = make(map[int]bool)
		for _, l := range lines {
			idx := atoi(l)
			subPopIndexMap[idx] = true
		}

	}

	vcfChan := readVCF(*vcfFileArg)
	p2arr := make([]float64, *maxlFlag)
	p2counts := make([]int64, *maxlFlag)
	var buffer []VCFRecord
	var currentChromo string
	for rec := range vcfChan {
		if rec.Pos < *regionStartFlag || rec.Pos > *regionEndFlag {
			break
		}
		if *chrom != "" && *chrom != rec.Chrom {
			continue
		}
		if (currentChromo == "" || currentChromo == rec.Chrom) && (len(buffer) == 0 || rec.Pos-buffer[0].Pos < *maxlFlag) {
			buffer = append(buffer, rec)
		} else {
			compute(buffer, p2arr, p2counts, subPopIndexMap)
			buffer = buffer[1:]
		}
	}
	compute(buffer, p2arr, p2counts, subPopIndexMap)

	w, err := os.Create(*outFileArg)
	if err != nil {
		panic(err)
	}
	defer w.Close()
	w.WriteString("l,m,n,v,t,b\n")
	for k := 0; k < len(p2arr); k++ {
		var m float64
		var n int64
		var t string
		n = p2counts[k]
		if k == 0 {
			m = p2arr[0] / float64(p2counts[0])
			t = "Ks"
		} else {
			m = p2arr[k] / p2arr[0]
			t = "P2"
		}
		if n > 0 {
			w.WriteString(fmt.Sprintf("%d,%g,0,%d,%s,all\n", k, m, n, t))
		}
	}
}

// Compute calculates correlation function.
func compute(buffer []VCFRecord, p2arr []float64, p2counts []int64, indexMap map[int]bool) {
	for i := 0; i < len(buffer); i++ {
		nc := mcorr.NewNuclCov([]byte{'0', '1', '2', '3'})
		if indexMap != nil {
			for k := range indexMap {
				a := buffer[0].GTs[k]
				b := buffer[i].GTs[k]
				if a-'0' >= 0 && a-'0' <= 3 && b-'0' >= 0 && b-'0' <= 3 {
					nc.Add(a, b)
				}
			}
		} else {
			for k := 0; k < len(buffer[0].GTs); k++ {
				a := buffer[0].GTs[k]
				b := buffer[i].GTs[k]
				if a-'0' >= 0 && a-'0' <= 3 && b-'0' >= 0 && b-'0' <= 3 {
					nc.Add(a, b)
				}
			}
		}
		lag := buffer[i].Pos - buffer[0].Pos
		xy, n := nc.P11(0)
		if n > len(buffer[0].GTs)/2 {
			p2arr[lag] += xy / float64(n)
			p2counts[lag]++
		}
	}
}

// readVCF return a channel of VCF record.
func readVCF(filename string) (c chan VCFRecord) {
	c = make(chan VCFRecord)
	go func() {
		defer close(c)
		f, err := os.Open(filename)
		if err != nil {
			panic(err)
		}
		defer f.Close()

		rd := bufio.NewReader(f)
		for {
			line, err := rd.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					panic(err)
				}
				break
			}
			if line[0] == '#' {
				continue
			}

			line = strings.TrimSpace(line)
			terms := strings.Split(line, "\t")
			var rec VCFRecord
			rec.Chrom = terms[0]
			rec.Pos = atoi(terms[1])
			rec.Ref = terms[3]
			rec.Alt = terms[4]
			if len(rec.Alt) == 1 && len(rec.Ref) == 1 {
				inGT := false
				for _, t := range terms {
					if t == "GT" {
						inGT = true
					} else if inGT {
						phased := true
						for _, gt := range t {
							if gt == '/' {
								phased = false
								break
							}
						}
						if phased {
							for _, gt := range t {
								if gt != '|' {
									rec.GTs = append(rec.GTs, byte(gt))
								}
							}
						} else {
							var current byte
							for _, gt := range t {
								if gt == '/' {
									continue
								}
								if current != 0 && current != byte(gt) {
									current = 0
									break
								}
								current = byte(gt)
							}
							if current != 0 {
								rec.GTs = append(rec.GTs, current)
							}
						}
						for _, gt := range t {
							if gt != '|' {
								rec.GTs = append(rec.GTs, byte(gt))
							}
						}
					}
				}
				c <- rec
			}
		}
	}()
	return
}

func atoi(s string) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		panic(err)
	}
	return v
}

func readTrimLines(filename string) []string {
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	var lines []string
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				panic(err)
			}
			break
		}
		lines = append(lines, strings.TrimSpace(line))
	}
	return lines
}
