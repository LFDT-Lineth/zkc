;; As permutation_03, but without selectors.
(defcolumns (X :i16) (Y :i32) (X2 :i17))
;; X2 holds the value of the expression (* X 2).
(defconstraint x2_def () (== X2 (* X 2)))
(defpermutation (test :unchecked) ((_ X2)) ((_ Y)))
