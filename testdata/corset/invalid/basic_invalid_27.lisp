;;error:3:22-118:constant value exceeds field modulus
(module m1)
(defcolumns (x :i16) (y :i256 :constant 8444461749428370424248824938781546531375899335154063827935233455917409239041))

(defconstraint c1 () (if (== x y) (== 0 x)))
