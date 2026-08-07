(module m1)
;;
(defcolumns (X_LO :i128) (X_HI :i128) (X :i256))
;; X holds the value of the expression (:: X_HI X_LO)
(defconstraint x_def () (== X (:: X_HI X_LO)))
;;
(deflookup l1 (m2.Y) (X))

(module m2)
(defcolumns (Y :i256))
