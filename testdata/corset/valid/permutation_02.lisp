;; Permutation between pairs of columns, where the columns within each pair
;; have different bitwidths.
(defcolumns (A :i16) (B :i8) (X :i16) (Y :i8) (one :i1 :constant 1))
(defpermutation test ((one A B)) ((one X Y)))
