package util

// Helper returns a formatted string.
func Helper(name string) string {
	if name == "" {
		return "default"
	}
	return "hello " + name
}

// Add returns the sum of two integers.
func Add(a, b int) int {
	return a + b
}

// Multiply returns the product of two integers.
func Multiply(a, b int) int {
	return a * b
}
