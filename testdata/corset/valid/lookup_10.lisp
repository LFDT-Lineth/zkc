(defun (selector) (* m1.X 2))

(module m1)
(defcolumns (X :i16) (Y :i17) (X2 :i17))
;; X2 holds the value of the expression (selector)
(defconstraint x2_def () (== X2 (selector)))
(deflookup test (Y) (X2))
