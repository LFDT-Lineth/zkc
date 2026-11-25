;; Permutation between pairs of columns, where one source column is defined by
;; an expression.
(defcolumns (A :i16) (B :i16) (X :i16) (Y :i18) (B3 :i18) (one :i1 :constant 1))
;; B3 holds the value of the expression (* B 3).
(defconstraint b3_def () (== B3 (* B 3)))
(defpermutation test ((one A B3)) ((one X Y)))
