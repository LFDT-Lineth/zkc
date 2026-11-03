;;error:6:28-29:constant P exceeds field modulus
(module m1)
(defconst P 8444461749428370424248824938781546531375899335154063827935233455917409239041)
(defcolumns (x :i16))

(defconstraint c1 () (== x P))
