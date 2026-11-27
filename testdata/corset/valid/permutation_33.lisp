;; As permutation_02, but without a source selector.
(defcolumns (A :i16) (B :i8) (X :i16) (Y :i8) (one :i1 :constant 1))
(defpermutation test ((_ A B)) ((one X Y)))
