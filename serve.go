package main

import (
    "fmt"
    "net/http"
)

const Small = 0.0000000001
const MaxTrys = 100;

func handler(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "<html><body>Hi there, I really love %s!  <br>", r.URL.Path[1:])
    fmt.Fprintf(w, "%s <br>", pSqrt(64))
    fmt.Fprintf(w, "%s <br>", pSqrt(16))
    fmt.Fprintf(w, "%s <br>", pSqrt(9))
    fmt.Fprintf(w, "%s <br>", pSqrt(4))
    fmt.Fprintf(w, "%s <br>", pSqrt(2))
    fmt.Fprintf(w, "%s <br>", pSqrt(-2))
    fmt.Fprintf(w, "</body></html>")
}

type ErrNegativeSqrt float64

func (e ErrNegativeSqrt) Error() string {
  return fmt.Sprintf("Hi i'm some kind of problem with %g", e)
}

func Newton(z float64, x float64) float64 {
  return z - ( ((z*z) - x) / (2.0 * z) );
}

func Abs(z float64) float64 {
  if z < 0 {
    return -z
  }
  return z
}

func AboutEqual(x float64, y float64) bool {
  return Abs(x - y) <= Small
}

func pSqrt(x float64) string {
  result, err := Sqrt(x);
  if err == nil {
    return fmt.Sprintf("%20.15g", result)
  }
  return fmt.Sprintf("Problem with %20.15g", x)
}

func Sqrt(x float64) (float64, error) {
  if x < 0 {
    return x, ErrNegativeSqrt(x)
  }
  var oldZ = 1.0;
  var newZ = Newton(oldZ, x)
  for count := 1; count < MaxTrys && !AboutEqual(oldZ, newZ); count++ {
    fmt.Println(count);
    oldZ = newZ;
    newZ = Newton(oldZ, x)
  }
  return newZ, nil
}

func main() {
    http.HandleFunc("/", handler)
    http.ListenAndServe(":8080", nil)
}
