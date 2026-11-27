;; Permutation whose source column is defined by a product of columns.
(defcolumns (ST :i4) (X :i16) (Y :i20) (STX :i20) (one :i1 :constant 1))
;; STX holds the value of the expression (* ST (+ 1 X)).
(defconstraint stx_def () (== STX (* ST (+ 1 X))))
(defpermutation test ((one STX)) ((one Y)))
