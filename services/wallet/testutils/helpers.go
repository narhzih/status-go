package testutils

import "reflect"

const EthSymbol = "ETH"
const SntSymbol = "SNT"
const DaiSymbol = "DAI"

func SliceContains[T comparable](slice []T, value T) bool {
	for _, v := range slice {
		if v == value {
			return true
		}
	}
	return false
}
func StructExistsInSlice[T any](target T, slice []T) bool {
	for _, item := range slice {
		if reflect.DeepEqual(target, item) {
			return true
		}
	}
	return false
}
