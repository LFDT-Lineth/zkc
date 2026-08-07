(module m1)
;;
(defcolumns (X :i128 :array [0:1]) (XC :i256))
;; XC holds the value of the expression (:: [X 1] [X 0])
(defconstraint xc_def () (== XC (:: [X 1] [X 0])))
;;
(deflookup l1 (m2.Y) (XC))

(module m2)
(defcolumns (Y :i256))
